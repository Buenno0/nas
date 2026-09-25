package sincro

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"path"
	"path/filepath"
	"strings"
	"time"

	"nas/internal/cloud"
	"nas/internal/db"
	"nas/internal/scan"
)

// EsperaPeloWorker é quanto o Mac espera por um job antes de preparar ele
// mesmo: sem worker publicado (imagem ausente no ECR, Spot sem capacidade), o
// pedido ficaria na fila para sempre e o player, esperando.
const EsperaPeloWorker = 15 * time.Minute

// WorkerVisto diz se algum worker já respondeu um job neste Mac. Antes disso,
// mandar preparo para a nuvem é apostar numa fila que talvez ninguém leia.
func (m *Motor) WorkerVisto(ctx context.Context) bool {
	return m.db.EstadoNuvem(ctx, "worker_visto") != ""
}

// ErrSemProcessamento: a nuvem não tem fila de jobs configurada.
var ErrSemProcessamento = errors.New("processamento na nuvem não configurado (nuvem.fila_jobs)")

// PedirProcessamento manda o worker gerar os derivados de um item da nuvem.
// Idempotente do lado do Mac: um pedido pendente não é repetido.
func (m *Motor) PedirProcessamento(ctx context.Context, fileID int64) error {
	cfg := m.cfg()
	if cfg.FilaJobs == "" {
		return ErrSemProcessamento
	}
	arm, nctx, cancel, ok := m.chave.Vincular(ctx)
	if !ok {
		return ErrSoHibrido
	}
	defer cancel()
	filas, ok := arm.(cloud.Mensageria)
	if !ok {
		return ErrSemProcessamento
	}
	f, err := m.db.FileByID(ctx, fileID)
	if err != nil {
		return err
	}
	if f.NuvemKey == "" {
		return ErrEstado
	}
	// Pedido recente: não repete. Um pedido velho sem resposta é refeito (o
	// worker pode ter sido publicado depois).
	if estado, _, quando := m.db.ProcessamentoDesde(ctx, fileID); estado == "pedido" && time.Since(quando) < EsperaPeloWorker {
		return nil
	}
	corpo, _ := json.Marshal(cloud.Job{SchemaVersion: cloud.VersaoDoContrato, Tipo: "processar", Key: f.NuvemKey, Origem: "mac"})
	if err := filas.EnviarMensagem(nctx, cfg.FilaJobs, corpo); err != nil {
		m.db.Anota(ctx, "job.erro", 0, fileID, filepath.Base(f.RelPath), map[string]any{"erro": err.Error()})
		return err
	}
	m.db.Anota(ctx, "job.pedido", 0, fileID, filepath.Base(f.RelPath), map[string]any{"key": f.NuvemKey})
	return m.db.MarcaProcessamento(ctx, fileID, "pedido", "")
}

// ChegouNaNuvem é chamado quando um arquivo acaba de chegar ao bucket. O
// evento do S3 já pôs o job na fila; aqui o pedido fica registrado (a página
// do título mostra "preparando") e o worker é acordado na hora, sem esperar o
// autoscaling notar a fila.
func (m *Motor) ChegouNaNuvem(ctx context.Context, fileID int64) {
	cfg := m.cfg()
	if cfg.FilaJobs == "" || fileID == 0 {
		return
	}
	_ = m.db.MarcaProcessamento(ctx, fileID, "pedido", "")
	arm, nctx, cancel, ok := m.chave.Vincular(ctx)
	if !ok {
		return
	}
	defer cancel()
	ac, ok := arm.(cloud.Acordador)
	if !ok {
		return
	}
	cluster, servico := "ozymandias", "ozymandias-worker"
	if c, s, ok := strings.Cut(cfg.Worker, "/"); ok {
		cluster, servico = c, s
	}
	if err := ac.AcordarWorker(nctx, cluster, servico); err != nil {
		log.Printf("acordando o worker: %v", err)
		return
	}
	m.db.Anota(ctx, "job.acordou", 0, fileID, "", map[string]any{"servico": cluster + "/" + servico})
}

// consumirCatalogo lê a assinatura do Mac no tópico catalogo enquanto o
// híbrido durar. Long-poll de 20 s: barato, e o kill switch o corta na hora.
func (m *Motor) consumirCatalogo(ctx context.Context) {
	fila := m.cfg().FilaEventos
	if fila == "" {
		return
	}
	arm, nctx, cancel, ok := m.chave.Vincular(ctx)
	if !ok {
		return
	}
	defer cancel()
	filas, ok := arm.(cloud.Mensageria)
	if !ok {
		return
	}
	for nctx.Err() == nil {
		msgs, err := filas.ReceberMensagens(nctx, fila, 10, 20*time.Second)
		if err != nil {
			if nctx.Err() == nil {
				log.Printf("fila do catálogo: %v", err)
				time.Sleep(10 * time.Second)
			}
			continue
		}
		for _, msg := range msgs {
			if err := m.AplicarEvento(nctx, arm, msg.Corpo); err != nil {
				log.Printf("evento do catálogo: %v", err)
				if msg.Recebimentos < 5 {
					continue // tenta de novo; depois disso a DLQ fica com ele
				}
			}
			_ = filas.ApagarMensagem(nctx, fila, msg.Recibo)
		}
	}
}

// AplicarEvento traduz um evento da nuvem para o catálogo local. Eventos nunca
// apagam nada: no máximo acrescentam derivados e metadados.
func (m *Motor) AplicarEvento(ctx context.Context, arm cloud.Armazenamento, corpo []byte) error {
	// Sem raw delivery, o SNS embrulha a mensagem num envelope.
	var envelope struct {
		Type    string `json:"Type"`
		Message string `json:"Message"`
	}
	if json.Unmarshal(corpo, &envelope) == nil && envelope.Type == "Notification" {
		corpo = []byte(envelope.Message)
	}
	// Evento de outro nó (outbox) ou do worker (job.*)?
	var fed cloud.EventoFederado
	if err := json.Unmarshal(corpo, &fed); err == nil && fed.EventID != "" {
		if fed.SchemaVersion > cloud.VersaoDoContrato {
			log.Printf("evento %s com schema_version %d: ignorado", idDoEvento(fed), fed.SchemaVersion)
			return nil
		}
		return m.aplicarFederado(ctx, arm, fed)
	}
	var ev cloud.EventoDoCatalogo
	if err := json.Unmarshal(corpo, &ev); err != nil {
		return err
	}
	if ev.SchemaVersion > cloud.VersaoDoContrato {
		log.Printf("evento %s com schema_version %d, este Mac entende até %d: ignorado",
			ev.Tipo, ev.SchemaVersion, cloud.VersaoDoContrato)
		return nil
	}

	fileID, err := m.db.FileIDPorNuvemKey(ctx, ev.Key)
	if errors.Is(err, db.ErrNotFound) && ev.Tipo == "job.concluido" {
		fileID, err = m.importarChave(ctx, ev.Key)
	}
	if err != nil {
		return err
	}
	if fileID == 0 {
		return nil // chave fora de qualquer biblioteca daqui
	}

	// Qualquer resposta prova que existe um worker do outro lado da fila.
	if ev.Tipo == "job.concluido" || ev.Tipo == "job.falhou" {
		_ = m.db.GravaEstadoNuvem(ctx, "worker_visto", time.Now().Format(time.RFC3339))
		dados := map[string]any{"key": ev.Key}
		if ev.Erro != "" {
			dados["erro"] = ev.Erro
		}
		// Quanto o job levou, do pedido deste Mac até a resposta.
		if estado, _, quando := m.db.ProcessamentoDesde(ctx, fileID); estado == "pedido" && !quando.IsZero() {
			dados["ms"] = time.Since(quando).Milliseconds()
		}
		if ev.Manifesto != nil {
			tipos := make([]string, 0, len(ev.Manifesto.Derivados))
			for _, d := range ev.Manifesto.Derivados {
				tipos = append(tipos, d.Tipo)
			}
			dados["derivados"] = tipos
		}
		m.db.Anota(ctx, ev.Tipo, 0, fileID, path.Base(ev.Key), dados)
	}
	switch ev.Tipo {
	case "job.falhou":
		return m.db.MarcaProcessamento(ctx, fileID, "falhou", ev.Erro)
	case "job.concluido":
		if ev.Manifesto == nil {
			return errors.New("job.concluido sem manifesto")
		}
		if len(ev.Manifesto.Probe) > 0 {
			var p scan.ProbeResult
			if err := json.Unmarshal(ev.Manifesto.Probe, &p); err == nil {
				if err := scan.New(m.db).AplicarProbe(ctx, fileID, p); err != nil {
					return err
				}
			}
		}
		ds := make([]db.Derivado, 0, len(ev.Manifesto.Derivados))
		for _, d := range ev.Manifesto.Derivados {
			ds = append(ds, db.Derivado{Tipo: d.Tipo, Indice: d.Indice, Key: d.Key, Receita: d.Receita})
		}
		return m.db.GravaDerivados(ctx, fileID, ds)
	}
	return nil
}

// importarChave coloca no catálogo um objeto que o Mac ainda não conhecia
// (enviado com ele dormindo, por exemplo). Sem ffprobe: o manifesto traz.
func (m *Motor) importarChave(ctx context.Context, key string) (int64, error) {
	libID, rel, ok := Interpretar(key)
	if !ok {
		return 0, nil
	}
	lib, err := m.db.Library(ctx, libID)
	if err != nil {
		return 0, nil
	}
	arm, _, ok := m.chave.Hibrido()
	if !ok {
		return 0, ErrSoHibrido
	}
	obj, err := arm.Info(ctx, key)
	if err != nil {
		return 0, err
	}
	return scan.New(m.db).IndexarNuvem(ctx, lib, key, rel, obj.Tamanho, "")
}
