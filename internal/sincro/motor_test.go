package sincro

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"nas/internal/cloud"
	"nas/internal/config"
	"nas/internal/db"
	"nas/internal/scan"
)

// memoria é um bucket S3 em memória, com ETag de multipart de verdade.
type memoria struct {
	mu      sync.Mutex
	objetos map[string]memObj
	envios  map[string]map[int32][]byte
	seq     int
	chave   *cloud.Chave
}

type memObj struct {
	dados []byte
	etag  string
	parte int64
}

// toca simula a rede: no modo local, o guard recusaria.
func (m *memoria) toca(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.chave.Modo() == cloud.ModoLocal {
		return cloud.ErrModoLocal
	}
	return nil
}

func (m *memoria) Verificar(context.Context) error { return nil }
func (m *memoria) Tamanho(ctx context.Context, key string) (int64, error) {
	o, err := m.Info(ctx, key)
	return o.Tamanho, err
}
func (m *memoria) Info(ctx context.Context, key string) (cloud.Objeto, error) {
	if err := m.toca(ctx); err != nil {
		return cloud.Objeto{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objetos[key]
	if !ok {
		return cloud.Objeto{}, cloud.ErrNaoExiste
	}
	return cloud.Objeto{Key: key, Tamanho: int64(len(o.dados)), ETag: `"` + o.etag + `"`, TamanhoParte: o.parte}, nil
}
func (m *memoria) Listar(_ context.Context, prefixo string, fn func(cloud.Objeto) error) error {
	m.mu.Lock()
	var keys []string
	for k := range m.objetos {
		if strings.HasPrefix(k, prefixo) {
			keys = append(keys, k)
		}
	}
	m.mu.Unlock()
	sort.Strings(keys)
	for _, k := range keys {
		o, _ := m.Info(context.Background(), k)
		if err := fn(o); err != nil {
			return err
		}
	}
	return nil
}
func (m *memoria) Baixar(ctx context.Context, key string, desde int64) (io.ReadCloser, error) {
	if err := m.toca(ctx); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objetos[key]
	if !ok {
		return nil, cloud.ErrNaoExiste // como o S3 (NoSuchKey)
	}
	return io.NopCloser(bytes.NewReader(o.dados[desde:])), nil
}
func (m *memoria) Gravar(ctx context.Context, key string, corpo []byte, _ string) error {
	if err := m.toca(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := md5.Sum(corpo)
	m.objetos[key] = memObj{dados: corpo, etag: hex.EncodeToString(s[:])}
	return nil
}
func (m *memoria) Apagar(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objetos, key)
	return nil
}
func (m *memoria) URLDeLeitura(_ context.Context, key string, _ time.Duration) (string, error) {
	return "", nil // sem URL: o ffprobe não roda nos testes
}
func (m *memoria) IniciarEnvio(context.Context, string, string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := fmt.Sprint(m.seq)
	m.envios[id] = map[int32][]byte{}
	return id, nil
}
func (m *memoria) URLDaParte(context.Context, string, string, int32, time.Duration) (string, error) {
	return "", nil
}
func (m *memoria) EnviarParte(ctx context.Context, _, id string, n int32, corpo io.ReadSeeker, _ int64) (string, error) {
	if err := m.toca(ctx); err != nil {
		return "", err
	}
	b, _ := io.ReadAll(corpo)
	if err := m.toca(ctx); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.envios[id][n] = b
	s := md5.Sum(b)
	return `"` + hex.EncodeToString(s[:]) + `"`, nil
}
func (m *memoria) PartesEnviadas(_ context.Context, _, id string) ([]cloud.Parte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ps []cloud.Parte
	for n, b := range m.envios[id] {
		s := md5.Sum(b)
		ps = append(ps, cloud.Parte{Numero: n, ETag: hex.EncodeToString(s[:])})
	}
	return ps, nil
}
func (m *memoria) ConcluirEnvio(_ context.Context, key, id string, partes []cloud.Parte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var dados, md5s []byte
	var parte int64
	for i, p := range partes {
		b := m.envios[id][p.Numero]
		if i == 0 {
			parte = int64(len(b))
		}
		dados = append(dados, b...)
		s := md5.Sum(b)
		md5s = append(md5s, s[:]...)
	}
	s := md5.Sum(md5s)
	m.objetos[key] = memObj{dados: dados, etag: fmt.Sprintf("%s-%d", hex.EncodeToString(s[:]), len(partes)), parte: parte}
	delete(m.envios, id)
	return nil
}
func (m *memoria) AbortarEnvio(_ context.Context, _, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.envios, id)
	return nil
}

type ambiente struct {
	db    *db.DB
	chave *cloud.Chave
	mem   *memoria
	motor *Motor
	lib   db.Library
}

func montar(t *testing.T) *ambiente {
	t.Helper()
	database, err := db.OpenAt(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	lib, err := database.AddLibrary(context.Background(), "Filmes", t.TempDir(), db.KindMovie)
	if err != nil {
		t.Fatal(err)
	}

	mem := &memoria{objetos: map[string]memObj{}, envios: map[string]map[int32][]byte{}}
	cloud.Registrar(func(context.Context, config.Nuvem, *http.Client) (cloud.Armazenamento, error) {
		return mem, nil
	})
	chave := cloud.Nova(func() config.Nuvem { return config.Nuvem{Bucket: "b", Regiao: "r"} }, false)
	mem.chave = chave
	if err := chave.Ativar(context.Background()); err != nil {
		t.Fatal(err)
	}
	m := Novo(database, chave, func() int64 { return 0 }, func() config.Nuvem { return config.Nuvem{} })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go m.Rodar(ctx)
	return &ambiente{db: database, chave: chave, mem: mem, motor: m, lib: lib}
}

// esperar aguarda a fila esvaziar.
func (a *ambiente) esperar(t *testing.T) {
	t.Helper()
	prazo := time.Now().Add(5 * time.Second)
	for time.Now().Before(prazo) {
		e := a.motor.Estado(context.Background())
		ocupado := e.Reconciliando
		for _, tr := range e.Tarefas {
			if tr.Estado == "erro" {
				t.Fatalf("tarefa %s falhou: %s", tr.Tipo, tr.Erro)
			}
			ocupado = ocupado || tr.Estado != "pausado"
		}
		if !ocupado {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("a fila não esvaziou")
}

func (a *ambiente) arquivoLocal(t *testing.T, nome string, tamanho int) db.MediaFile {
	t.Helper()
	caminho := filepath.Join(a.lib.Path, nome)
	dados := bytes.Repeat([]byte("ozymandias"), tamanho/10+1)[:tamanho]
	if err := os.WriteFile(caminho, dados, 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := a.db.UpsertFile(context.Background(), db.MediaFile{
		LibraryID: a.lib.ID, Path: caminho, RelPath: nome, Ext: filepath.Ext(nome),
		Size: int64(tamanho), MTime: 1, Type: db.TypeVideo,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := a.db.FileByID(context.Background(), id)
	return f
}

func (a *ambiente) loc(t *testing.T, id int64) db.MediaFile {
	t.Helper()
	f, err := a.db.FileByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// Ciclo completo: local → ambos → liberar espaço (nuvem) → fixar (ambos),
// com o conteúdo idêntico no fim.
func TestCicloDeLocalizacao(t *testing.T) {
	a := montar(t)
	ctx := context.Background()
	f := a.arquivoLocal(t, "Duna (2021).mkv", 40<<20) // 3 partes de 16 MiB
	original, _ := os.ReadFile(f.Path)

	if err := a.motor.Enviar(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	a.esperar(t)
	if g := a.loc(t, f.ID); g.Localizacao != db.LocalAmbos || g.NuvemKey == "" {
		t.Fatalf("depois de enviar: %s %q", g.Localizacao, g.NuvemKey)
	}

	if err := a.motor.Liberar(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	a.esperar(t)
	g := a.loc(t, f.ID)
	if g.Localizacao != db.LocalNuvem {
		t.Fatalf("depois de liberar: %s", g.Localizacao)
	}
	if _, err := os.Stat(f.Path); !os.IsNotExist(err) {
		t.Fatal("liberar espaço não apagou a cópia local")
	}

	// O scan não pode tirar do catálogo o que agora mora só na nuvem.
	if _, err := scan.New(a.db).ScanLibrary(ctx, a.lib, nil); err != nil {
		t.Fatal(err)
	}
	a.loc(t, f.ID)

	if err := a.motor.Fixar(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	a.esperar(t)
	g = a.loc(t, f.ID)
	if g.Localizacao != db.LocalAmbos || g.Path != f.Path {
		t.Fatalf("depois de fixar: %s em %s", g.Localizacao, g.Path)
	}
	baixado, _ := os.ReadFile(f.Path)
	if !bytes.Equal(baixado, original) {
		t.Fatal("o arquivo fixado difere do original")
	}
}

// Liberar espaço com uma cópia local diferente da nuvem não apaga nada.
func TestLiberarRecusaCopiaDiferente(t *testing.T) {
	a := montar(t)
	ctx := context.Background()
	f := a.arquivoLocal(t, "x.mkv", 1<<20)
	if err := a.motor.Enviar(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	a.esperar(t)
	os.WriteFile(f.Path, bytes.Repeat([]byte("z"), 1<<20), 0o644) // editado depois do envio

	if err := a.motor.Liberar(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	prazo := time.Now().Add(3 * time.Second)
	for time.Now().Before(prazo) {
		e := a.motor.Estado(ctx)
		if len(e.Tarefas) == 1 && e.Tarefas[0].Estado == "erro" {
			if _, err := os.Stat(f.Path); err != nil {
				t.Fatal("a cópia local foi apagada mesmo diferente")
			}
			if a.loc(t, f.ID).Localizacao != db.LocalAmbos {
				t.Fatal("a localização mudou apesar da recusa")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("liberar deveria ter falhado com ErrDiferente")
}

// Reconciliação: drena o outbox para o journal e importa do bucket o que o
// catálogo não conhecia.
func TestReconciliacao(t *testing.T) {
	a := montar(t)
	ctx := context.Background()
	a.db.RegistraEvento(ctx, "teste", map[string]int{"x": 1})

	key := fmt.Sprintf("bibliotecas/%d/abc123/Alien (1979).mkv", a.lib.ID)
	a.mem.Gravar(ctx, key, []byte("filme"), "")
	a.mem.Gravar(ctx, "bibliotecas/999/zzz/sem-biblioteca.mkv", []byte("x"), "")

	a.motor.Reconciliar(ctx)
	a.esperar(t)

	if n, _ := a.db.ContaEventos(ctx); n != 0 {
		t.Fatalf("outbox não drenou: %d eventos", n)
	}
	var journal int
	a.mem.Listar(ctx, "eventos/mac/", func(cloud.Objeto) error { journal++; return nil })
	if journal == 0 {
		t.Fatal("nenhum journal gravado no bucket")
	}
	conhecidas, _ := a.db.ChavesNaNuvem(ctx)
	if !conhecidas[key] {
		t.Fatal("o objeto do bucket não entrou no catálogo")
	}
	if len(conhecidas) != 1 {
		t.Fatalf("importou chave de biblioteca inexistente: %v", conhecidas)
	}
}

// Kill switch no meio de um envio: pausa, e ao voltar o híbrido a
// reconciliação retoma até o fim.
func TestKillSwitchPausaERetoma(t *testing.T) {
	a := montar(t)
	ctx := context.Background()
	f := a.arquivoLocal(t, "grande.mkv", 64<<20)

	a.chave.Desligar()
	if err := a.motor.Enviar(ctx, f.ID); err != ErrSoHibrido {
		t.Fatalf("no modo local, Enviar deveria recusar: %v", err)
	}
	if err := a.chave.Ativar(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.motor.Enviar(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	for a.loc(t, f.ID).Localizacao != db.LocalEnviando {
		time.Sleep(time.Millisecond)
	}
	a.chave.Desligar()
	time.Sleep(50 * time.Millisecond)
	if g := a.loc(t, f.ID); g.Localizacao == db.LocalAmbos {
		t.Skip("o envio terminou antes do kill switch; máquina rápida demais para este teste")
	}

	if err := a.chave.Ativar(ctx); err != nil {
		t.Fatal(err)
	}
	a.motor.Reconciliar(ctx)
	a.esperar(t)
	if g := a.loc(t, f.ID); g.Localizacao != db.LocalAmbos {
		t.Fatalf("depois de retomar: %s", g.Localizacao)
	}
}

// job.concluido: grava derivados e o probe do manifesto; aceita o envelope
// do SNS; importa a chave se o Mac ainda não a conhecia.
func TestAplicarEventoDoCatalogo(t *testing.T) {
	a := montar(t)
	ctx := context.Background()
	key := fmt.Sprintf("bibliotecas/%d/abc/Solaris (1972).mkv", a.lib.ID)
	a.mem.Gravar(ctx, key, []byte("filme"), "")

	probe, _ := json.Marshal(scan.ProbeResult{Duration: 9000, VCodec: "hevc", Width: 1920, Height: 1080,
		Streams: []scan.Stream{{Index: 2, Kind: "subtitle", Codec: "subrip", Lang: "por"}}})
	ev, _ := json.Marshal(cloud.EventoDoCatalogo{SchemaVersion: 1, Tipo: "job.concluido", Key: key,
		Manifesto: &cloud.Manifesto{SchemaVersion: 1, Key: key, Probe: probe, Derivados: []cloud.Derivado{
			{Tipo: "compat", Key: cloud.PrefixoDosDerivados(key) + "compat.mp4", Receita: "video1080"},
			{Tipo: "legenda", Indice: 2, Key: cloud.PrefixoDosDerivados(key) + "legenda-2.vtt"},
		}}})
	envelope, _ := json.Marshal(map[string]string{"Type": "Notification", "Message": string(ev)})

	arm, _, _ := a.chave.Hibrido()
	if err := a.motor.AplicarEvento(ctx, arm, envelope); err != nil {
		t.Fatal(err)
	}
	id, err := a.db.FileIDPorNuvemKey(ctx, key)
	if err != nil {
		t.Fatalf("a chave desconhecida não foi importada: %v", err)
	}
	if _, err := a.db.DerivadoDe(ctx, id, "compat", 0); err != nil {
		t.Fatal("derivado compat não gravado")
	}
	if _, err := a.db.DerivadoDe(ctx, id, "legenda", 2); err != nil {
		t.Fatal("derivado de legenda não gravado")
	}
	f := a.loc(t, id)
	if f.Duration != 9000 || f.VCodec != "hevc" {
		t.Fatalf("probe do manifesto não aplicado: %+v", f)
	}
	if estado, _ := a.db.Processamento(ctx, id); estado != "concluido" {
		t.Fatalf("processamento = %q", estado)
	}

	falhou, _ := json.Marshal(cloud.EventoDoCatalogo{SchemaVersion: 1, Tipo: "job.falhou", Key: key, Erro: "ffmpeg morreu"})
	a.motor.AplicarEvento(ctx, arm, falhou)
	if estado, erro := a.db.Processamento(ctx, id); estado != "falhou" || erro != "ffmpeg morreu" {
		t.Fatalf("job.falhou: %q %q", estado, erro)
	}

	// Versão futura do contrato: ignorada, sem erro e sem estrago.
	futuro, _ := json.Marshal(cloud.EventoDoCatalogo{SchemaVersion: 99, Tipo: "job.falhou", Key: key})
	if err := a.motor.AplicarEvento(ctx, arm, futuro); err != nil {
		t.Fatal(err)
	}
}

// Apagar um objeto direto no bucket (console, CLI): o item só da nuvem sai do
// catálogo, o que também está no Mac volta a ser só local, e o que continua
// no bucket fica intacto.
func TestApagadoPorForaDoBucket(t *testing.T) {
	a := montar(t)
	ctx := context.Background()
	arm, _, _ := a.chave.Hibrido()
	grava := func(key string) {
		if err := arm.Gravar(ctx, key, []byte("x"), ""); err != nil {
			t.Fatal(err)
		}
	}

	soNuvem := a.arquivoLocal(t, "Solaris (1972).mkv", 100)
	a.db.MarcaNaNuvem(ctx, soNuvem.ID, db.LocalNuvem, "bibliotecas/1/a/Solaris (1972).mkv")
	ambos := a.arquivoLocal(t, "Stalker (1979).mkv", 100)
	a.db.MarcaNaNuvem(ctx, ambos.ID, db.LocalAmbos, "bibliotecas/1/b/Stalker (1979).mkv")
	fica := a.arquivoLocal(t, "Espelho (1975).mkv", 100)
	a.db.MarcaNaNuvem(ctx, fica.ID, db.LocalNuvem, "bibliotecas/1/c/Espelho (1975).mkv")
	grava("bibliotecas/1/c/Espelho (1975).mkv") // só este continua no bucket

	// Direto, sem a trava: a reconciliação de fundo (ao entrar no híbrido)
	// pode estar rodando e adiaria esta.
	a.motor.reconciliarUmaVez(ctx)

	if _, err := a.db.FileByID(ctx, soNuvem.ID); err == nil {
		t.Fatal("item só da nuvem apagado do bucket continuou no catálogo")
	}
	if f := a.loc(t, ambos.ID); f.Localizacao != db.LocalLocal || f.NuvemKey != "" {
		t.Fatalf("item com cópia no Mac: %s %q, quero local", f.Localizacao, f.NuvemKey)
	}
	if f := a.loc(t, fica.ID); f.Localizacao != db.LocalNuvem {
		t.Fatalf("item que continua no bucket mudou para %s", f.Localizacao)
	}
}
