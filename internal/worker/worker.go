// Package worker é o processamento na nuvem: roda em container (ECS Fargate
// Spot), consome a fila de jobs e gera, para cada objeto do bucket, os
// derivados que o Mac não consegue gerar para um item que ele não tem no
// disco — MP4 compatível, miniaturas, legendas em WebVTT.
//
// O resultado vai para derivados/<hash>/ e é anunciado no tópico catalogo.
// O Mac consome quando está no híbrido; com ele dormindo, o worker continua.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nas/internal/cloud"
	"nas/internal/db"
	"nas/internal/media"
	"nas/internal/scan"
	"nas/internal/sincro"
)

// Config vem de variáveis de ambiente no container.
type Config struct {
	FilaJobs string
	Topico   string
	Dir      string // área de trabalho (armazenamento efêmero do Fargate)
	// MaxTentativas antes de anunciar job.falhou. A DLQ da fila recebe a
	// mensagem depois disso por conta própria (redrive policy).
	MaxTentativas int
	// UmaVez sai quando a fila esvazia: útil para testar e para rodar como
	// task avulsa em vez de service.
	UmaVez bool
}

const (
	visibilidade = 5 * time.Minute
	pulso        = 2 * time.Minute
)

// Rodar consome a fila até ctx acabar.
func Rodar(ctx context.Context, arm cloud.Armazenamento, cfg Config) error {
	filas, ok := arm.(cloud.Mensageria)
	if !ok {
		return errors.New("o adapter de nuvem não tem filas")
	}
	if cfg.MaxTentativas <= 0 {
		cfg.MaxTentativas = 3
	}
	vazias := 0
	for ctx.Err() == nil {
		msgs, err := filas.ReceberMensagens(ctx, cfg.FilaJobs, 1, 20*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("recebendo da fila: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		if len(msgs) == 0 {
			vazias++
			if cfg.UmaVez && vazias >= 2 {
				return nil
			}
			continue
		}
		vazias = 0
		for _, m := range msgs {
			tratar(ctx, arm, filas, cfg, m)
		}
	}
	return nil
}

func tratar(ctx context.Context, arm cloud.Armazenamento, filas cloud.Mensageria, cfg Config, m cloud.Mensagem) {
	jobs, err := cloud.JobsDaMensagem(m.Corpo)
	if err != nil {
		log.Printf("mensagem %s ilegível, descartada: %v", m.ID, err)
		_ = filas.ApagarMensagem(ctx, cfg.FilaJobs, m.Recibo)
		return
	}

	// Segura a mensagem enquanto trabalha: um vídeo de 2 h leva mais que a
	// visibilidade padrão, e outra réplica não pode pegá-lo no meio.
	jctx, parar := context.WithCancel(ctx)
	go func() {
		t := time.NewTicker(pulso)
		defer t.Stop()
		for {
			select {
			case <-jctx.Done():
				return
			case <-t.C:
				_ = filas.EstenderVisibilidade(jctx, cfg.FilaJobs, m.Recibo, visibilidade)
			}
		}
	}()
	defer parar()

	for _, j := range jobs {
		inicio := time.Now()
		man, err := Processar(jctx, arm, cfg.Dir, j.Key)
		evento := cloud.EventoDoCatalogo{SchemaVersion: cloud.VersaoDoContrato, Key: j.Key}
		if err != nil {
			log.Printf("job %s falhou (tentativa %d): %v", j.Key, m.Recebimentos, err)
			if m.Recebimentos < cfg.MaxTentativas {
				return // volta à fila quando a visibilidade expirar
			}
			evento.Tipo, evento.Erro = "job.falhou", err.Error()
		} else {
			log.Printf("job %s concluído em %s: %d derivados", j.Key, time.Since(inicio).Round(time.Second), len(man.Derivados))
			evento.Tipo, evento.Manifesto = "job.concluido", &man
		}
		corpo, _ := json.Marshal(evento)
		if err := filas.Publicar(ctx, cfg.Topico, corpo, "worker"); err != nil {
			log.Printf("publicando %s: %v", evento.Tipo, err)
			return // sem anúncio, o job é refeito: melhor que um Mac desinformado
		}
	}
	if err := filas.ApagarMensagem(ctx, cfg.FilaJobs, m.Recibo); err != nil {
		log.Printf("apagando mensagem %s: %v", m.ID, err)
	}
}

// Processar gera os derivados de um objeto e grava o manifesto.
func Processar(ctx context.Context, arm cloud.Armazenamento, dir, key string) (cloud.Manifesto, error) {
	mtype, ok := scan.TipoPorExtensao(key)
	if !ok {
		return cloud.Manifesto{}, fmt.Errorf("%s não é mídia", key)
	}
	obj, err := arm.Info(ctx, key)
	if err != nil {
		return cloud.Manifesto{}, err
	}

	trabalho, err := os.MkdirTemp(dir, "job-*")
	if err != nil {
		return cloud.Manifesto{}, err
	}
	defer os.RemoveAll(trabalho)

	ext := strings.ToLower(filepath.Ext(key))
	origem := filepath.Join(trabalho, "original"+ext)
	if err := baixar(ctx, arm, key, origem); err != nil {
		return cloud.Manifesto{}, fmt.Errorf("baixando: %w", err)
	}

	man := cloud.Manifesto{SchemaVersion: cloud.VersaoDoContrato, Key: key,
		ETag: strings.Trim(obj.ETag, `"`), Tamanho: obj.Tamanho, ProcessadoEm: time.Now().UTC()}
	prefixo := cloud.PrefixoDosDerivados(key)
	sobe := func(tipo, caminho, nome, contentType string, indice int, receita string) error {
		k := prefixo + nome
		if _, err := sincro.EnviarCaminho(ctx, arm, k, caminho, contentType); err != nil {
			return fmt.Errorf("enviando %s: %w", nome, err)
		}
		man.Derivados = append(man.Derivados, cloud.Derivado{Tipo: tipo, Indice: indice, Key: k, Receita: receita})
		return nil
	}
	imagens := filepath.Join(trabalho, "imagens")

	var probe scan.ProbeResult
	if mtype != db.TypePhoto {
		if probe, err = scan.Probe(ctx, origem); err != nil {
			return cloud.Manifesto{}, err
		}
		man.Probe, _ = json.Marshal(probe)
	}

	switch mtype {
	case db.TypePhoto:
		for _, w := range []int{320, 800} {
			nome, err := media.ImageThumb(ctx, origem, 0, imagens, w)
			if err != nil {
				return cloud.Manifesto{}, err
			}
			if err := sobe(fmt.Sprintf("thumb%d", w), filepath.Join(imagens, nome), fmt.Sprintf("thumb%d.jpg", w), "image/jpeg", 0, ""); err != nil {
				return cloud.Manifesto{}, err
			}
		}

	case db.TypeAudio:
		if nome, err := media.EmbeddedCover(ctx, origem, 0, imagens); err == nil {
			if err := sobe("capa", filepath.Join(imagens, nome), "capa.jpg", "image/jpeg", 0, ""); err != nil {
				return cloud.Manifesto{}, err
			}
		}

	case db.TypeVideo:
		if nome, err := media.VideoFrame(ctx, origem, 0, probe.Duration, imagens); err == nil {
			if err := sobe("frame", filepath.Join(imagens, nome), "frame.jpg", "image/jpeg", 0, ""); err != nil {
				return cloud.Manifesto{}, err
			}
		}

		// Legendas de texto viram WebVTT; as de imagem (PGS) não têm conversão.
		prep := media.NovoPreparador(filepath.Join(trabalho, "preparo"), 1<<50, 0, 1)
		for _, st := range probe.Streams {
			if st.Kind != "subtitle" || media.EhLegendaDeImagem(st.Codec) {
				continue
			}
			vtt, err := prep.LegendaVTT(ctx, media.PedidoLegenda{Origem: origem, Codec: st.Codec, Indice: st.Index})
			if err != nil {
				log.Printf("legenda %d de %s: %v", st.Index, key, err)
				continue
			}
			if err := sobe("legenda", vtt, fmt.Sprintf("legenda-%d.vtt", st.Index), "text/vtt", st.Index, ""); err != nil {
				return cloud.Manifesto{}, err
			}
		}

		// O MP4 compatível só existe quando o original não toca num
		// navegador comum: o direto continua sendo lido do próprio objeto.
		// Com uma faixa AAC no mesmo idioma da padrão, ela entra no MP4 e o
		// áudio não é recodificado: só reembalar, bem mais rápido.
		acodec, audio, trocou := probe.ACodec, -1, false
		var faixas []media.FaixaDeAudio
		for _, st := range probe.Streams {
			if st.Kind == string(db.StreamAudio) {
				faixas = append(faixas, media.FaixaDeAudio{Index: st.Index, Codec: st.Codec, Lang: st.Lang, Default: st.Default})
			}
		}
		if alt, ok := media.AudioAlternativo(faixas, media.Caps{}); ok {
			acodec, audio, trocou = alt.Codec, alt.Index, true
		}
		plano := media.DecideWithSize(ext, probe.VCodec, probe.PixFmt, probe.VProfile, acodec,
			probe.Width, probe.Height, trocou, media.Caps{})
		if plano.Mode != media.ModeDirect {
			pedido := media.Pedido{Origem: origem, Duracao: probe.Duration, Receita: plano.Recipe, Audio: audio}
			if _, err := prep.Pedir(ctx, pedido); err != nil {
				return cloud.Manifesto{}, fmt.Errorf("preparando (%s): %w", plano.Recipe, err)
			}
			pronto, err := prep.Esperar(ctx, pedido)
			if err != nil {
				return cloud.Manifesto{}, fmt.Errorf("preparando (%s): %w", plano.Recipe, err)
			}
			if pronto.Estado != media.EstadoPronto {
				return cloud.Manifesto{}, fmt.Errorf("preparo terminou em %s: %s", pronto.Estado, pronto.Erro)
			}
			if err := sobe("compat", pronto.Arquivo, "compat.mp4", "video/mp4", 0, plano.Recipe); err != nil {
				return cloud.Manifesto{}, err
			}
		}
	}

	corpo, _ := json.MarshalIndent(man, "", "  ")
	if err := arm.Gravar(ctx, prefixo+"manifesto.json", corpo, "application/json"); err != nil {
		return cloud.Manifesto{}, err
	}
	return man, nil
}

func baixar(ctx context.Context, arm cloud.Armazenamento, key, destino string) error {
	corpo, err := arm.Baixar(ctx, key, 0)
	if err != nil {
		return err
	}
	defer corpo.Close()
	f, err := os.Create(destino)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, corpo); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
