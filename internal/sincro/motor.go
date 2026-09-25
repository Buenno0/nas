package sincro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"nas/internal/cloud"
	"nas/internal/config"
	"nas/internal/db"
	"nas/internal/media"
	"nas/internal/scan"
)

// Tarefa é um trabalho em andamento, para a interface mostrar progresso.
type Tarefa struct {
	FileID int64  `json:"file_id"`
	Tipo   string `json:"tipo"` // enviar | fixar | liberar | remover
	Nome   string `json:"nome"`
	Feitos int64  `json:"feitos"`
	Total  int64  `json:"total"`
	Estado string `json:"estado"` // fila | trabalhando | pausado | erro
	Erro   string `json:"erro,omitempty"`
}

// Estado é o que /api/sincronizacao devolve.
type Estado struct {
	Tarefas          []Tarefa `json:"tarefas"`
	EventosPendentes int64    `json:"eventos_pendentes"`
	UltimaReconc     string   `json:"ultima_reconciliacao,omitempty"`
	Reconciliando    bool     `json:"reconciliando"`
	Erro             string   `json:"erro,omitempty"`
}

// Motor executa uma operação de rede por vez. O link do Mac é finito e o
// disco tem 13 GB livres: paralelismo aqui só disputaria banda e espaço.
type Motor struct {
	db      *db.DB
	chave   *cloud.Chave
	reserva func() int64
	cfg     func() config.Nuvem

	mu            sync.Mutex
	tarefas       map[string]*Tarefa
	fila          chan trabalho
	reconciliando bool
	// pendente: pediram outra reconciliação enquanto uma rodava. Ela roda de
	// novo ao terminar, em vez de o pedido se perder.
	pendente bool
	erro     string
}

type trabalho struct {
	id  string
	run func(ctx context.Context, arm cloud.Armazenamento, t *Tarefa) error
}

// Novo cria o motor. reserva é o espaço livre que fixar nunca consome.
func Novo(database *db.DB, chave *cloud.Chave, reserva func() int64, cfg func() config.Nuvem) *Motor {
	return &Motor{db: database, chave: chave, reserva: reserva, cfg: cfg,
		tarefas: map[string]*Tarefa{}, fila: make(chan trabalho, 1024)}
}

var (
	ErrSoHibrido = errors.New("só no modo híbrido")
	ErrEstado    = errors.New("o arquivo não está no estado certo para isso")
	ErrSemEspaco = errors.New("sem espaço livre no disco para fixar este arquivo")
	ErrDiferente = errors.New("a cópia na nuvem não é idêntica à local: nada foi apagado")
	ErrJaNaFila  = errors.New("já há uma operação em andamento para este arquivo")
)

// Rodar é o laço do motor: executa a fila e reconcilia ao entrar no híbrido
// e a cada 15 minutos enquanto ele durar.
func (m *Motor) Rodar(ctx context.Context) {
	ch, sair := m.chave.Assinar()
	defer sair()
	tick := time.NewTicker(15 * time.Minute)
	defer tick.Stop()

	// A fila roda à parte: um download de 4 GB não pode atrasar a reação a
	// uma mudança de modo.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tr := <-m.fila:
				m.executar(ctx, tr)
			}
		}
	}()

	if m.chave.Modo() == cloud.ModoHibrido {
		go m.Reconciliar(ctx)
		go m.consumirCatalogo(ctx)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-ch:
			if e.Modo == cloud.ModoHibrido {
				go m.Reconciliar(ctx)
				go m.consumirCatalogo(ctx)
			} else if e.Modo == cloud.ModoLocal {
				m.pausarTudo()
			}
		case <-tick.C:
			if m.chave.Modo() == cloud.ModoHibrido {
				go m.Reconciliar(ctx)
			}
		}
	}
}

func (m *Motor) executar(ctx context.Context, tr trabalho) {
	m.mu.Lock()
	t := m.tarefas[tr.id]
	m.mu.Unlock()
	if t == nil {
		return
	}
	arm, nctx, cancel, ok := m.chave.Vincular(ctx)
	if !ok {
		m.marca(t, "pausado", "")
		return
	}
	defer cancel()
	m.marca(t, "trabalhando", "")
	err := tr.run(nctx, arm, t)
	switch {
	case err == nil:
		m.mu.Lock()
		delete(m.tarefas, tr.id)
		m.mu.Unlock()
	case m.chave.Modo() != cloud.ModoHibrido || errors.Is(err, context.Canceled):
		// Kill switch no meio: o estado no banco permite retomar.
		m.marca(t, "pausado", "")
	default:
		log.Printf("sincronização %s de %s: %v", t.Tipo, t.Nome, err)
		m.marca(t, "erro", err.Error())
	}
}

func (m *Motor) marca(t *Tarefa, estado, erro string) {
	m.mu.Lock()
	t.Estado, t.Erro = estado, erro
	m.mu.Unlock()
}

func (m *Motor) progresso(t *Tarefa) func(int64) {
	return func(n int64) {
		m.mu.Lock()
		t.Feitos = n
		m.mu.Unlock()
	}
}

func (m *Motor) pausarTudo() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tarefas {
		if t.Estado == "trabalhando" || t.Estado == "fila" {
			t.Estado = "pausado"
		}
	}
}

// enfileirar registra a tarefa e a manda para a fila. Uma por arquivo.
func (m *Motor) enfileirar(t Tarefa, run func(context.Context, cloud.Armazenamento, *Tarefa) error) error {
	id := fmt.Sprintf("%s:%d", t.Tipo, t.FileID)
	m.mu.Lock()
	if atual, ok := m.tarefas[id]; ok && atual.Estado != "erro" && atual.Estado != "pausado" {
		m.mu.Unlock()
		return ErrJaNaFila
	}
	t.Estado = "fila"
	m.tarefas[id] = &t
	m.mu.Unlock()
	select {
	case m.fila <- trabalho{id: id, run: run}:
		return nil
	default:
		return errors.New("fila de sincronização cheia")
	}
}

// Estado resume o que está acontecendo.
func (m *Motor) Estado(ctx context.Context) Estado {
	m.mu.Lock()
	e := Estado{Reconciliando: m.reconciliando, Erro: m.erro, Tarefas: []Tarefa{}}
	for _, t := range m.tarefas {
		e.Tarefas = append(e.Tarefas, *t)
	}
	m.mu.Unlock()
	e.EventosPendentes, _ = m.db.ContaEventos(ctx)
	e.UltimaReconc = m.db.EstadoNuvem(ctx, "ultima_reconciliacao")
	return e
}

// --- Enviar (local → ambos) -------------------------------------------------

// Enviar dá a um arquivo local uma cópia na nuvem. Retomável: se já existe um
// upload aberto para ele, continua de onde parou.
func (m *Motor) Enviar(ctx context.Context, fileID int64) error {
	if m.chave.Modo() != cloud.ModoHibrido {
		return ErrSoHibrido
	}
	f, err := m.db.FileByID(ctx, fileID)
	if err != nil {
		return err
	}
	if f.Localizacao != db.LocalLocal && f.Localizacao != db.LocalEnviando {
		return ErrEstado
	}
	return m.enfileirar(Tarefa{FileID: fileID, Tipo: "enviar", Nome: filepath.Base(f.RelPath), Total: f.Size},
		func(ctx context.Context, arm cloud.Armazenamento, t *Tarefa) error {
			return m.enviar(ctx, arm, f, t)
		})
}

func (m *Motor) enviar(ctx context.Context, arm cloud.Armazenamento, f db.MediaFile, t *Tarefa) error {
	u, err := m.db.UploadPendenteDe(ctx, f.Path)
	if err == nil && u.Tamanho != f.Size {
		_ = arm.AbortarEnvio(ctx, u.Key, u.UploadID)
		_ = m.db.EstadoDoUpload(ctx, u.ID, "abortado")
		err = db.ErrNotFound
	}
	if err != nil {
		key, err := ChaveDoUpload(f.LibraryID, f.RelPath)
		if err != nil {
			return err
		}
		u = db.Upload{LibraryID: f.LibraryID, Key: key, Nome: f.RelPath, Tamanho: f.Size,
			ParteTamanho: TamanhoDaParte(f.Size), Origem: f.Path, MediaFileID: &f.ID}
		if u.UploadID, err = arm.IniciarEnvio(ctx, key, ""); err != nil {
			return err
		}
		if u.ID, err = m.db.CriaUpload(ctx, u); err != nil {
			_ = arm.AbortarEnvio(ctx, key, u.UploadID)
			return err
		}
	}
	_ = m.db.MarcaNaNuvem(ctx, f.ID, db.LocalEnviando, "")
	nome := filepath.Base(f.RelPath)
	m.db.Anota(ctx, "envio.inicio", u.ID, f.ID, nome, map[string]any{
		"origem": "mac", "tamanho": u.Tamanho, "parte_tamanho": u.ParteTamanho, "key": u.Key})

	partes, err := EnviarArquivoCom(ctx, arm, u, f.Path, m.progresso(t), m.anotaParte(u.ID, f.ID, nome))
	if err != nil {
		_ = m.db.MarcaNaNuvem(context.WithoutCancel(ctx), f.ID, db.LocalLocal, "")
		m.anotaInterrupcao(ctx, u.ID, f.ID, nome, err)
		return err
	}
	obj, err := Concluir(ctx, arm, u, partes)
	if err != nil {
		m.db.Anota(ctx, "envio.erro", u.ID, f.ID, nome, map[string]any{"erro": err.Error()})
		return err
	}
	m.db.Anota(ctx, "envio.concluido", u.ID, f.ID, nome, map[string]any{
		"origem": "mac", "partes": len(partes), "etag": strings.Trim(obj.ETag, `"`), "tamanho": obj.Tamanho})
	if err := m.db.EstadoDoUpload(ctx, u.ID, "concluido"); err != nil {
		return err
	}
	if err := m.db.MarcaNaNuvem(ctx, f.ID, db.LocalAmbos, u.Key); err != nil {
		return err
	}
	return m.db.MudaCaminho(ctx, f.ID, f.Path, db.LocalAmbos, strings.Trim(obj.ETag, `"`), f.MTime)
}

func (m *Motor) anotaParte(uploadID, fileID int64, nome string) AoEnviarParte {
	return func(p cloud.Parte, tamanho int64, d time.Duration) {
		m.db.Anota(context.Background(), "envio.parte", uploadID, fileID, nome, map[string]any{
			"n": p.Numero, "tamanho": tamanho, "ms": d.Milliseconds(), "etag": strings.Trim(p.ETag, `"`)})
	}
}

// anotaInterrupcao separa a pausa do kill switch de um erro de verdade.
func (m *Motor) anotaInterrupcao(ctx context.Context, uploadID, fileID int64, nome string, err error) {
	if m.chave.Modo() != cloud.ModoHibrido || errors.Is(err, context.Canceled) {
		m.db.Anota(ctx, "envio.pausa", uploadID, fileID, nome, map[string]any{"motivo": "kill switch"})
		return
	}
	m.db.Anota(ctx, "envio.erro", uploadID, fileID, nome, map[string]any{"erro": err.Error()})
}

// --- Fixar (nuvem → baixando → ambos) ---------------------------------------

// Fixar baixa um item da nuvem para a pasta da biblioteca ("disponível
// offline"). Retomável: o parcial fica ao lado do destino.
func (m *Motor) Fixar(ctx context.Context, fileID int64) error {
	if m.chave.Modo() != cloud.ModoHibrido {
		return ErrSoHibrido
	}
	f, err := m.db.FileByID(ctx, fileID)
	if err != nil {
		return err
	}
	if !db.SoNaNuvem(f.Localizacao) {
		return ErrEstado
	}
	lib, err := m.db.Library(ctx, f.LibraryID)
	if err != nil {
		return err
	}
	destino := f.Path
	if f.Localizacao == db.LocalNuvem {
		destino = filepath.Join(lib.Path, filepath.FromSlash(f.RelPath))
	}
	if livre, err := media.EspacoLivre(lib.Path); err == nil && livre-f.Size < m.reserva() {
		return ErrSemEspaco
	}
	if _, err := os.Stat(destino); err == nil && f.Localizacao == db.LocalNuvem {
		return fmt.Errorf("já existe um arquivo em %s", f.RelPath)
	}
	// O caminho real entra no índice ANTES do download: se o scan passar pela
	// pasta no meio, ele reconhece a linha (baixando) em vez de criar outra.
	if f.Localizacao == db.LocalNuvem {
		if err := m.db.MudaCaminho(ctx, f.ID, destino, db.LocalBaixando, "", f.MTime); err != nil {
			return err
		}
	}
	return m.enfileirar(Tarefa{FileID: fileID, Tipo: "fixar", Nome: filepath.Base(f.RelPath), Total: f.Size},
		func(ctx context.Context, arm cloud.Armazenamento, t *Tarefa) error {
			return m.fixar(ctx, arm, f, destino, t)
		})
}

func (m *Motor) fixar(ctx context.Context, arm cloud.Armazenamento, f db.MediaFile, destino string, t *Tarefa) error {
	obj, err := arm.Info(ctx, f.NuvemKey)
	if err != nil {
		return err
	}
	parcial := destino + ".ozy-parcial"
	if err := os.MkdirAll(filepath.Dir(destino), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(parcial, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	desde, err := out.Seek(0, io.SeekEnd)
	if err != nil || desde > obj.Tamanho {
		out.Close()
		return fmt.Errorf("parcial inválido em %s", parcial)
	}
	prog := m.progresso(t)
	prog(desde)
	if desde < obj.Tamanho {
		corpo, err := arm.Baixar(ctx, f.NuvemKey, desde)
		if err != nil {
			out.Close()
			return err
		}
		_, err = io.Copy(out, &contador{r: corpo, n: desde, f: prog})
		corpo.Close()
		if err != nil {
			out.Close()
			return err
		}
	}
	if err := out.Close(); err != nil {
		return err
	}

	// Conferência antes de assumir a cópia local como verdade.
	etag, err := cloud.ETagLocal(parcial, obj.TamanhoParte)
	if err != nil {
		return err
	}
	if !cloud.MesmoConteudo(etag, obj.ETag) {
		os.Remove(parcial)
		return fmt.Errorf("download corrompido (ETag %s ≠ %s): o parcial foi descartado", etag, obj.ETag)
	}
	if err := os.Rename(parcial, destino); err != nil {
		return err
	}
	info, err := os.Stat(destino)
	if err != nil {
		return err
	}
	if err := m.db.MudaCaminho(ctx, f.ID, destino, db.LocalAmbos, etag, info.ModTime().Unix()); err != nil {
		return err
	}
	m.db.RegistraEvento(ctx, "localizacao.alterada", map[string]any{
		"file_id": f.ID, "localizacao": db.LocalAmbos, "key": f.NuvemKey,
	})
	return nil
}

type contador struct {
	r io.Reader
	n int64
	f func(int64)
}

func (c *contador) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	c.f(c.n)
	return n, err
}

// --- Liberar espaço (ambos → nuvem) -----------------------------------------

// Liberar apaga a cópia local de um item que também está na nuvem, depois de
// provar pelo ETag que as duas são idênticas. É uma das duas únicas ações
// destrutivas, e só acontece por pedido explícito na UI ou na CLI.
func (m *Motor) Liberar(ctx context.Context, fileID int64) error {
	if m.chave.Modo() != cloud.ModoHibrido {
		return ErrSoHibrido
	}
	f, err := m.db.FileByID(ctx, fileID)
	if err != nil {
		return err
	}
	if f.Localizacao != db.LocalAmbos || f.NuvemKey == "" {
		return ErrEstado
	}
	return m.enfileirar(Tarefa{FileID: fileID, Tipo: "liberar", Nome: filepath.Base(f.RelPath), Total: f.Size},
		func(ctx context.Context, arm cloud.Armazenamento, t *Tarefa) error {
			obj, err := arm.Info(ctx, f.NuvemKey)
			if err != nil {
				return err
			}
			etag, err := cloud.ETagLocal(f.Path, obj.TamanhoParte)
			if err != nil {
				return err
			}
			if !cloud.MesmoConteudo(etag, obj.ETag) {
				return ErrDiferente
			}
			// Índice primeiro: se o processo cair entre os dois passos, o pior
			// caso é um arquivo que o próximo scan reindexa, nunca um item
			// perdido do catálogo.
			if err := m.db.MudaCaminho(ctx, f.ID, scan.CaminhoNuvem(f.NuvemKey), db.LocalNuvem, etag, f.MTime); err != nil {
				return err
			}
			if err := os.Remove(f.Path); err != nil {
				_ = m.db.MudaCaminho(context.WithoutCancel(ctx), f.ID, f.Path, db.LocalAmbos, etag, f.MTime)
				return err
			}
			m.db.RegistraEvento(ctx, "localizacao.alterada", map[string]any{
				"file_id": f.ID, "localizacao": db.LocalNuvem, "key": f.NuvemKey,
			})
			t.Feitos = t.Total
			return nil
		})
}

// RemoverDaNuvem apaga a cópia do bucket de um item que continua no Mac. A
// outra ação destrutiva; também só por pedido explícito, nunca por evento.
func (m *Motor) RemoverDaNuvem(ctx context.Context, fileID int64) error {
	if m.chave.Modo() != cloud.ModoHibrido {
		return ErrSoHibrido
	}
	f, err := m.db.FileByID(ctx, fileID)
	if err != nil {
		return err
	}
	if f.Localizacao != db.LocalAmbos || f.NuvemKey == "" {
		return ErrEstado
	}
	if _, err := os.Stat(f.Path); err != nil {
		return fmt.Errorf("a cópia local sumiu; remover da nuvem apagaria o único exemplar")
	}
	return m.enfileirar(Tarefa{FileID: fileID, Tipo: "remover", Nome: filepath.Base(f.RelPath)},
		func(ctx context.Context, arm cloud.Armazenamento, t *Tarefa) error {
			if err := arm.Apagar(ctx, f.NuvemKey); err != nil {
				return err
			}
			return m.db.MarcaNaNuvem(ctx, f.ID, db.LocalLocal, "")
		})
}

// --- Reconciliação ----------------------------------------------------------

// Reconciliar é o que acontece ao entrar no híbrido: drena o outbox para o
// journal, importa do bucket o que o catálogo não conhece, retoma envios,
// fixações e espelhamentos pendentes.
func (m *Motor) Reconciliar(ctx context.Context) {
	m.mu.Lock()
	if m.reconciliando {
		m.pendente = true
		m.mu.Unlock()
		return
	}
	m.reconciliando = true
	m.mu.Unlock()
	for {
		m.reconciliarUmaVez(ctx)
		m.mu.Lock()
		if !m.pendente || m.chave.Modo() != cloud.ModoHibrido {
			m.pendente = false
			m.reconciliando = false
			m.mu.Unlock()
			return
		}
		m.pendente = false
		m.mu.Unlock()
	}
}

func (m *Motor) reconciliarUmaVez(ctx context.Context) {
	arm, nctx, cancel, ok := m.chave.Vincular(ctx)
	if !ok {
		return
	}
	defer cancel()

	var erros []string
	passo := func(nome string, err error) {
		if err != nil && nctx.Err() == nil {
			log.Printf("reconciliação (%s): %v", nome, err)
			erros = append(erros, nome+": "+err.Error())
		}
	}
	passo("catálogo", m.importarDoBucket(nctx, arm))
	if m.papelNuvem() {
		// A instância cloud não tem disco de mídia: nada a retomar nem a
		// espelhar. O estado do Mac chega pelo snapshot.
		passo("snapshot", m.carregarSnapshot(nctx, arm))
	} else {
		passo("retomadas", m.retomar(nctx))
		passo("espelho", m.espelhar(nctx))
		passo("snapshot", m.publicarSnapshot(nctx, arm))
	}
	// Por último: os passos acima também geram eventos.
	passo("journal", m.drenarOutbox(nctx, arm))

	if nctx.Err() != nil {
		return // kill switch: a próxima entrada no híbrido refaz tudo
	}
	m.mu.Lock()
	m.erro = strings.Join(erros, "; ")
	m.mu.Unlock()
	_ = m.db.GravaEstadoNuvem(ctx, "ultima_reconciliacao", time.Now().Format(time.RFC3339))
}

type linhaDoJournal struct {
	SchemaVersion int             `json:"schema_version"`
	Origem        string          `json:"origem"`
	ID            int64           `json:"id"`
	Tipo          string          `json:"tipo"`
	Payload       json.RawMessage `json:"payload"`
	Criado        int64           `json:"criado_ms"`
}

// drenarOutbox grava os eventos em lotes JSONL em eventos/mac/. O nome do
// objeto é o primeiro id do lote: reenviar depois de uma queda sobrescreve o
// mesmo objeto em vez de duplicar eventos.
func (m *Motor) drenarOutbox(ctx context.Context, arm cloud.Armazenamento) error {
	for {
		evs, err := m.db.EventosPendentes(ctx, 1000)
		if err != nil || len(evs) == 0 {
			return err
		}
		var b strings.Builder
		enc := json.NewEncoder(&b)
		for _, e := range evs {
			if err := enc.Encode(linhaDoJournal{SchemaVersion: db.VersaoDosEventos, Origem: m.cfg().OrigemDosEventos(),
				ID: e.ID, Tipo: e.Tipo, Payload: e.Payload, Criado: e.Criado}); err != nil {
				return err
			}
		}
		key := fmt.Sprintf("eventos/%s/%020d.jsonl", m.cfg().OrigemDosEventos(), evs[0].ID)
		if err := arm.Gravar(ctx, key, []byte(b.String()), "application/x-ndjson"); err != nil {
			return err
		}
		if err := m.publicarEventos(ctx, arm, evs); err != nil {
			return err
		}
		ultimo := evs[len(evs)-1].ID
		if err := m.db.ConfirmaEventos(ctx, ultimo); err != nil {
			return err
		}
		_ = m.db.GravaEstadoNuvem(ctx, "ultimo_evento_enviado", strconv.FormatInt(ultimo, 10))
	}
}

// importarDoBucket traz ao catálogo objetos de bibliotecas/ que ele não
// conhece: enviados de outro lugar, ou de antes de um banco perdido.
func (m *Motor) importarDoBucket(ctx context.Context, arm cloud.Armazenamento) error {
	conhecidas, err := m.db.ChavesNaNuvem(ctx)
	if err != nil {
		return err
	}
	libs := map[int64]db.Library{}
	todas, err := m.db.Libraries(ctx)
	if err != nil {
		return err
	}
	for _, l := range todas {
		libs[l.ID] = l
	}
	sc := scan.New(m.db)
	return arm.Listar(ctx, "bibliotecas/", func(o cloud.Objeto) error {
		if conhecidas[o.Key] {
			return nil
		}
		libID, rel, ok := Interpretar(o.Key)
		lib, existe := libs[libID]
		if !ok || !existe {
			return nil
		}
		if _, ok := scan.TipoPorExtensao(rel); !ok {
			return nil
		}
		leitura, err := arm.URLDeLeitura(ctx, o.Key, time.Hour)
		if err != nil {
			return err
		}
		if _, err := sc.IndexarNuvem(ctx, lib, o.Key, rel, o.Tamanho, leitura); err != nil {
			log.Printf("importando %s: %v", o.Key, err)
		}
		return nil
	})
}

// retomar reabre o que o kill switch (ou uma queda) interrompeu.
func (m *Motor) retomar(ctx context.Context) error {
	baixando, err := m.db.ArquivosPorLocalizacao(ctx, db.LocalBaixando, false)
	if err != nil {
		return err
	}
	for _, a := range baixando {
		if err := m.Fixar(ctx, a.ID); err != nil && !errors.Is(err, ErrJaNaFila) {
			log.Printf("retomando fixação de %s: %v", a.RelPath, err)
		}
	}
	enviando, err := m.db.ArquivosPorLocalizacao(ctx, db.LocalEnviando, false)
	if err != nil {
		return err
	}
	for _, a := range enviando {
		if err := m.Enviar(ctx, a.ID); err != nil && !errors.Is(err, ErrJaNaFila) {
			log.Printf("retomando envio de %s: %v", a.RelPath, err)
		}
	}
	// Envios do `nas push` interrompidos cujo arquivo já está no índice.
	ups, err := m.db.UploadsPendentes(ctx)
	if err != nil {
		return err
	}
	for _, u := range ups {
		if u.Origem != "" && u.MediaFileID != nil {
			if err := m.Enviar(ctx, *u.MediaFileID); err != nil && !errors.Is(err, ErrJaNaFila) && !errors.Is(err, ErrEstado) {
				log.Printf("retomando push de %s: %v", u.Nome, err)
			}
		}
	}
	return nil
}

// espelhar põe na fila todo arquivo local das bibliotecas espelhadas.
func (m *Motor) espelhar(ctx context.Context) error {
	locais, err := m.db.ArquivosPorLocalizacao(ctx, db.LocalLocal, true)
	if err != nil {
		return err
	}
	for _, a := range locais {
		if err := m.Enviar(ctx, a.ID); err != nil && !errors.Is(err, ErrJaNaFila) {
			return err
		}
	}
	return nil
}

// DefinirEspelho liga ou desliga o espelhamento. Ligar no híbrido já põe o
// acervo da biblioteca na fila; no local, espera o híbrido.
func (m *Motor) DefinirEspelho(ctx context.Context, libID int64, on bool) error {
	if err := m.db.SetEspelhada(ctx, libID, on); err != nil {
		return err
	}
	if on && m.chave.Modo() == cloud.ModoHibrido {
		return m.espelhar(ctx)
	}
	return nil
}
