package sincro

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nas/internal/cloud"
	"nas/internal/config"
	"nas/internal/db"
)

const (
	chaveDoSnapshot  = "catalogo/snapshots/mac.json.gz"
	prefixoDosPoster = "catalogo/posters/"
	// Um item da nuvem criado aqui há menos que isto e ausente do snapshot é
	// um upload que o Mac ainda não viu, não uma remoção.
	margemDoSnapshot = time.Hour
)

func (m *Motor) papelNuvem() bool { return m.cfg().Papel == config.PapelNuvem }

// publicarEventos manda os eventos do outbox para o tópico catalogo, um a
// um, com a origem deste nó. Sem tópico configurado, só o journal existe.
func (m *Motor) publicarEventos(ctx context.Context, arm cloud.Armazenamento, evs []db.Evento) error {
	cfg := m.cfg()
	filas, ok := arm.(cloud.Mensageria)
	if cfg.Topico == "" || !ok {
		return nil
	}
	origem := cfg.OrigemDosEventos()
	for _, e := range evs {
		corpo, _ := json.Marshal(cloud.EventoFederado{
			SchemaVersion: cloud.VersaoDoContrato, EventID: fmt.Sprintf("%s-%d", origem, e.ID),
			Origem: origem, Tipo: e.Tipo, Payload: e.Payload, CriadoMs: e.Criado,
		})
		if err := filas.Publicar(ctx, cfg.Topico, corpo, origem); err != nil {
			return err
		}
	}
	return nil
}

// publicarSnapshot grava o estado do Mac no bucket quando ele mudou, junto com
// os pôsteres que a instância cloud ainda não tem, e avisa pelo tópico.
func (m *Motor) publicarSnapshot(ctx context.Context, arm cloud.Armazenamento) error {
	snap, err := m.db.ExportarSnapshot(ctx)
	if err != nil {
		return err
	}
	// O hash ignora a hora de geração: snapshot igual não é republicado.
	quando := snap.GeradoEm
	snap.GeradoEm = time.Time{}
	cru, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	soma := sha256.Sum256(cru)
	hash := hex.EncodeToString(soma[:])
	if m.db.EstadoNuvem(ctx, "snapshot_hash") == hash {
		return nil
	}
	snap.GeradoEm = quando
	cru, _ = json.Marshal(snap)

	if err := m.enviarPosters(ctx, arm, snap); err != nil {
		return fmt.Errorf("pôsteres: %w", err)
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write(cru)
	gz.Close()
	if err := arm.Gravar(ctx, chaveDoSnapshot, buf.Bytes(), "application/gzip"); err != nil {
		return err
	}
	if err := m.db.GravaEstadoNuvem(ctx, "snapshot_hash", hash); err != nil {
		return err
	}
	// O aviso passa pelo outbox: publicado junto com os outros eventos.
	m.db.RegistraEvento(ctx, "snapshot.publicado", map[string]any{"key": chaveDoSnapshot, "hash": hash})
	return nil
}

func (m *Motor) enviarPosters(ctx context.Context, arm cloud.Armazenamento, snap db.Snapshot) error {
	dir, err := config.PosterDir()
	if err != nil {
		return err
	}
	tem := map[string]bool{}
	if err := arm.Listar(ctx, prefixoDosPoster, func(o cloud.Objeto) error {
		tem[strings.TrimPrefix(o.Key, prefixoDosPoster)] = true
		return nil
	}); err != nil {
		return err
	}
	for _, t := range snap.Titulos {
		for _, nome := range []string{t.Poster, t.Backdrop} {
			if nome == "" || tem[nome] || strings.ContainsAny(nome, `/\`) {
				continue
			}
			dados, err := os.ReadFile(filepath.Join(dir, nome))
			if err != nil {
				continue // pôster que sumiu do cache: a nuvem mostra o gradiente
			}
			if err := arm.Gravar(ctx, prefixoDosPoster+nome, dados, "image/jpeg"); err != nil {
				return err
			}
			tem[nome] = true
		}
	}
	return nil
}

// carregarSnapshot aplica, na instância cloud, o último snapshot do Mac se
// ele for novo.
func (m *Motor) carregarSnapshot(ctx context.Context, arm cloud.Armazenamento) error {
	obj, err := arm.Info(ctx, chaveDoSnapshot)
	if errors.Is(err, cloud.ErrNaoExiste) {
		return nil // o Mac ainda não publicou nada
	}
	if err != nil {
		return err
	}
	if m.db.EstadoNuvem(ctx, "snapshot_etag") == obj.ETag {
		return nil
	}
	corpo, err := arm.Baixar(ctx, chaveDoSnapshot, 0)
	if err != nil {
		return err
	}
	defer corpo.Close()
	gz, err := gzip.NewReader(corpo)
	if err != nil {
		return err
	}
	var snap db.Snapshot
	if err := json.NewDecoder(gz).Decode(&snap); err != nil {
		return err
	}
	if snap.SchemaVersion > db.VersaoDosEventos {
		return fmt.Errorf("snapshot com schema_version %d, esta instância entende até %d", snap.SchemaVersion, db.VersaoDosEventos)
	}
	res, err := m.db.ImportarSnapshot(db.SemEventos(ctx), snap, margemDoSnapshot)
	if err != nil {
		return err
	}
	log.Printf("snapshot do Mac aplicado: %d itens, %d removidos, %d usuários", res.Itens, res.Removidos, res.Usuarios)
	m.baixarPosters(ctx, arm, res.Posters)
	return m.db.GravaEstadoNuvem(ctx, "snapshot_etag", obj.ETag)
}

func (m *Motor) baixarPosters(ctx context.Context, arm cloud.Armazenamento, nomes []string) {
	dir, err := config.PosterDir()
	if err != nil {
		return
	}
	for _, nome := range nomes {
		if strings.ContainsAny(nome, `/\`) {
			continue
		}
		destino := filepath.Join(dir, nome)
		if _, err := os.Stat(destino); err == nil {
			continue
		}
		corpo, err := arm.Baixar(ctx, prefixoDosPoster+nome, 0)
		if err != nil {
			continue
		}
		dados, err := io.ReadAll(corpo)
		corpo.Close()
		if err == nil {
			_ = os.WriteFile(destino, dados, 0o644)
		}
	}
}

// aplicarFederado trata um evento do outbox de outro nó.
func (m *Motor) aplicarFederado(ctx context.Context, arm cloud.Armazenamento, ev cloud.EventoFederado) error {
	if ev.Origem == m.cfg().OrigemDosEventos() {
		return nil // o filtro da assinatura já deveria ter barrado
	}
	if repetido, err := m.db.JaProcessado(ctx, ev.EventID); err != nil || repetido {
		if repetido {
			m.db.Anota(ctx, "evento.repetido", 0, 0, ev.Tipo, map[string]any{"event_id": ev.EventID, "origem": ev.Origem})
		}
		return err
	}
	m.db.Anota(ctx, "evento.recebido", 0, 0, ev.Tipo, map[string]any{"event_id": ev.EventID, "origem": ev.Origem})
	ctx = db.SemEventos(ctx)
	switch ev.Tipo {
	case "progresso.atualizado":
		var p struct {
			UserID    int64   `json:"user_id"`
			Ref       db.Ref  `json:"ref"`
			Posicao   float64 `json:"posicao"`
			Duracao   float64 `json:"duracao"`
			UpdatedAt int64   `json:"updated_at"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return err
		}
		fileID, err := m.db.ArquivoPorRef(ctx, p.Ref)
		if err != nil {
			return nil // item que este nó não tem: nada a fazer
		}
		if _, err := m.db.UserByID(ctx, p.UserID); err != nil {
			return nil
		}
		return m.db.ProgressoLWW(ctx, p.UserID, fileID, p.Posicao, p.Duracao, p.UpdatedAt)

	case "favorito.alterado":
		var f struct {
			UserID   int64  `json:"user_id"`
			Ref      db.Ref `json:"ref"`
			Favorito bool   `json:"favorito"`
		}
		if err := json.Unmarshal(ev.Payload, &f); err != nil {
			return err
		}
		titleID, err := m.db.TituloPorRef(ctx, f.Ref)
		if err != nil {
			return nil
		}
		if _, err := m.db.UserByID(ctx, f.UserID); err != nil {
			return nil
		}
		return m.db.SetFavorite(ctx, f.UserID, titleID, f.Favorito)

	case "item.adicionado":
		var it struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(ev.Payload, &it); err != nil {
			return err
		}
		if _, err := m.db.FileIDPorNuvemKey(ctx, it.Key); errors.Is(err, db.ErrNotFound) {
			_, err = m.importarChave(ctx, it.Key)
			return err
		}
		return nil

	case "snapshot.publicado":
		if m.papelNuvem() {
			return m.carregarSnapshot(ctx, arm)
		}
	}
	// Outros tipos (localizacao.alterada, …) são informativos: o snapshot
	// seguinte já os traz.
	return nil
}

// idDoEvento só serve aos logs.
func idDoEvento(ev cloud.EventoFederado) string {
	if ev.EventID != "" {
		return ev.EventID
	}
	return ev.Tipo + "@" + strconv.FormatInt(ev.CriadoMs, 10)
}
