package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nas/internal/cloud"
	"nas/internal/db"
	"nas/internal/media"
	"nas/internal/sincro"
)

// A resposta que o player consulta antes de tocar qualquer coisa.
type respostaPlayback struct {
	Modo   media.Mode `json:"modo"`
	Motivo string     `json:"motivo"`
	// URL a usar agora. Em modo direto é o arquivo cru; nos outros, o preparado
	// (só válida quando preparo.estado == "pronto").
	URL string `json:"url"`
	// URLDireta permite ao player oferecer "tentar o original" e o download.
	URLDireta string           `json:"url_direta"`
	Preparo   *media.Progresso `json:"preparo,omitempty"`
	FFmpeg    bool             `json:"ffmpeg"`
	Ativo     bool             `json:"transcodificacao_ativa"`
	// Localizacao do arquivo; Indisponivel quando ele só existe na nuvem e o
	// modo é local.
	Localizacao  string `json:"localizacao"`
	Indisponivel bool   `json:"indisponivel,omitempty"`
}

// paramsDoPlano são os parâmetros que entram na escolha da receita — e,
// portanto, na chave do cache de preparo. Precisam viajar iguais por todas as
// quatro chamadas de uma reprodução (consultar o plano, pedir o preparo,
// acompanhar o progresso, buscar o pronto): divergir em um deles faria o
// player esperar por um preparo e pedir outro.
var paramsDoPlano = []string{"can", "vid", "aud", "cont", "audio", "maxw", "maxh"}

// maxItensDeCaps corta listas absurdas antes de virarem mapa. Nenhum cliente
// honesto declara cinquenta codecs.
const maxItensDeCaps = 40

// listaDaQuery quebra "aac,mp3, flac" em itens limpos.
func listaDaQuery(bruto string) []string {
	if bruto == "" {
		return nil
	}
	partes := strings.Split(bruto, ",")
	if len(partes) > maxItensDeCaps {
		partes = partes[:maxItensDeCaps]
	}
	itens := make([]string, 0, len(partes))
	for _, p := range partes {
		if p = strings.TrimSpace(p); p != "" {
			itens = append(itens, p)
		}
	}
	return itens
}

// capsDaQuery lê o que o cliente declarou saber tocar.
//
// Vem dele e não de farejar User-Agent: Safari toca HEVC por hardware, e
// mandá-lo para a transcodificação queimaria CPU à toa.
//
// Dois dialetos convivem aqui, e a diferença é o quanto o cliente sabe sobre
// si mesmo. O SPA manda ?can= com as três dúvidas que MediaSource responde e
// deixa o resto em branco — os padrões de navegador valem. Um cliente nativo
// manda ?vid=&aud=&cont= com os conjuntos inteiros, porque ele sabe
// exatamente o que a plataforma abre: o AVPlayer do iOS não toca Opus nem
// abre WebM, e nenhum padrão de navegador acerta isso por ele.
//
// Os três são independentes: declarar áudio não muda o veredito sobre vídeo.
func capsDaQuery(r *http.Request) media.Caps {
	q := r.URL.Query()

	var c media.Caps
	for _, item := range listaDaQuery(strings.ToLower(q.Get("can"))) {
		switch media.NormalizaVCodec(item) {
		case "hevc":
			c.HEVC = true
		case "vp9":
			c.VP9 = true
		case "av1":
			c.AV1 = true
		}
	}

	c.Video = media.Conjunto(listaDaQuery(q.Get("vid")), media.NormalizaVCodec)
	c.Audio = media.Conjunto(listaDaQuery(q.Get("aud")), strings.ToLower)
	c.Containers = media.Conjunto(listaDaQuery(q.Get("cont")), media.NormalizaExt)
	c.MaxWidth = dimensaoDaQuery(q.Get("maxw"))
	c.MaxHeight = dimensaoDaQuery(q.Get("maxh"))
	return c
}

func dimensaoDaQuery(value string) int {
	n, err := strconv.Atoi(value)
	if err != nil || n < 144 || n > 16384 {
		return 0
	}
	return n
}

// faixaEscolhida lê ?audio=N e o valida contra o banco.
//
// A validação não é burocracia: esse número vira argumento de -map do ffmpeg.
// Aceitar o que o cliente mandar seria deixá-lo apontar para qualquer stream do
// arquivo — ou para nenhum, e o preparo falharia sem explicação.
//
// Devolve (índice, codec, trocou). trocou=false quando a faixa pedida já é a
// padrão do arquivo: nesse caso não há motivo para reembalar nada.
func (s *Server) faixaEscolhida(r *http.Request, arquivo db.MediaFile) (int, string, bool) {
	bruto := r.URL.Query().Get("audio")
	if bruto == "" {
		// Sem escolha do usuário: se a faixa padrão não toca aqui mas existe
		// outra no mesmo idioma que toca, vai ela (reembalar em vez de
		// recodificar o áudio).
		if alt, ok := s.audioAlternativo(r.Context(), arquivo, capsDaQuery(r)); ok {
			return alt.Index, alt.Codec, true
		}
		return -1, arquivo.ACodec, false
	}
	idx, err := strconv.Atoi(bruto)
	if err != nil {
		return -1, arquivo.ACodec, false
	}
	faixa, err := s.db.StreamByIndex(r.Context(), arquivo.ID, idx)
	if err != nil || faixa.Kind != db.StreamAudio {
		return -1, arquivo.ACodec, false
	}
	return faixa.Index, faixa.Codec, !faixa.Default
}

func (s *Server) audioAlternativo(ctx context.Context, arquivo db.MediaFile, caps media.Caps) (media.FaixaDeAudio, bool) {
	streams, err := s.db.Streams(ctx, arquivo.ID)
	if err != nil {
		return media.FaixaDeAudio{}, false
	}
	var faixas []media.FaixaDeAudio
	for _, st := range streams {
		if st.Kind == db.StreamAudio {
			faixas = append(faixas, media.FaixaDeAudio{Index: st.Index, Codec: st.Codec, Lang: st.Lang, Default: st.Default})
		}
	}
	return media.AudioAlternativo(faixas, caps)
}

func (s *Server) planoDeArquivo(r *http.Request, arquivo db.MediaFile) (media.Plan, media.Pedido) {
	audio, acodec, trocou := s.faixaEscolhida(r, arquivo)
	plano := media.DecideWithSize(
		arquivo.Ext, arquivo.VCodec, arquivo.PixFmt, arquivo.VProfile, acodec,
		arquivo.Width, arquivo.Height, trocou, capsDaQuery(r),
	)
	pedido := media.Pedido{
		FileID:  arquivo.ID,
		Origem:  arquivo.Path,
		MTime:   arquivo.MTime,
		Duracao: arquivo.Duration,
		Receita: plano.Recipe,
		Audio:   audio,
	}
	// Só na nuvem: a origem é uma URL assinada, preenchida só na hora de
	// preparar (vale 6 h e muda a cada assinatura); o cache se orienta pela
	// identidade.
	if db.SoNaNuvem(arquivo.Localizacao) {
		pedido.Origem = ""
		pedido.Identidade = arquivo.Path
		pedido.Tamanho = arquivo.Size
	}
	return plano, pedido
}

// handlePlayback devolve o que tocar e em que estado está o preparo. Não começa
// trabalho nenhum: quem decide gastar CPU é o POST.
func (s *Server) handlePlayback(w http.ResponseWriter, r *http.Request) {
	arquivo, err := s.db.FileByID(r.Context(), atoi64(r.PathValue("id")))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "arquivo não encontrado")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	plano, pedido := s.planoDeArquivo(r, arquivo)
	resp := respostaPlayback{
		Modo:        plano.Mode,
		Motivo:      plano.Reason,
		URLDireta:   fmt.Sprintf("/stream/%d", arquivo.ID),
		FFmpeg:      s.ffmpegAvailable(),
		Ativo:       s.transcodeAtivo(),
		Localizacao: arquivo.Localizacao,
	}

	// Na instância cloud, item sem cópia no bucket mora só no Mac.
	if s.opts.NaNuvem && !db.SoNaNuvem(arquivo.Localizacao) {
		resp.Indisponivel = true
		resp.Motivo = "no Mac, indisponível na nuvem"
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Item só da nuvem: toca direto da CDN/bucket ou não toca.
	if db.SoNaNuvem(arquivo.Localizacao) {
		if s.nuvem.Modo() != cloud.ModoHibrido {
			resp.Indisponivel = true
			resp.Motivo = "na nuvem, indisponível no modo local"
			writeJSON(w, http.StatusOK, resp)
			return
		}
		if plano.Mode == media.ModeDirect {
			resp.URL = resp.URLDireta
			writeJSON(w, http.StatusOK, resp)
			return
		}
		if s.preparaNoWorker(r.Context(), arquivo) {
			s.respostaDaNuvem(r.Context(), &resp, arquivo, true)
			writeJSON(w, http.StatusOK, resp)
			return
		}
		// Sem worker: o Mac prepara lendo o original da nuvem. Daqui para
		// baixo é o mesmo fluxo de um arquivo do disco.
		resp.Motivo += " · preparando no Mac a partir da nuvem"
	}

	// Bursting: o arquivo também está no bucket e o Mac não está numa boa
	// hora (bateria, calor, fila cheia). O worker prepara; o player espera
	// igual, só que sem gastar a bateria.
	if plano.Mode != media.ModeDirect {
		if ok, porque := s.prepararNaNuvem(r.Context(), arquivo, pedido); ok {
			resp.Motivo += " · preparando na nuvem: " + porque
			s.respostaDaNuvem(r.Context(), &resp, arquivo, false)
			writeJSON(w, http.StatusOK, resp)
			return
		}
	}

	if plano.Mode == media.ModeDirect {
		resp.URL = resp.URLDireta
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Sem ffmpeg ou com transcodificação desligada, o player cai no aviso de
	// formato não suportado com o comando para converter à mão.
	if !resp.FFmpeg || !resp.Ativo {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	progresso := s.preparador.Consultar(pedido)
	resp.Preparo = &progresso
	if progresso.Estado == media.EstadoPronto {
		resp.URL = urlPreparado(arquivo.ID, r)
	}
	writeJSON(w, http.StatusOK, resp)
}

// urlPreparado repassa os parâmetros que participam da escolha da receita e,
// portanto, da chave do cache. Perder o ?audio aqui faria o player pedir de
// volta o preparo da faixa padrão — com a barra de progresso da outra.
func urlPreparado(id int64, r *http.Request) string {
	q := url.Values{}
	origem := r.URL.Query()
	for _, nome := range paramsDoPlano {
		if v := origem.Get(nome); v != "" {
			q.Set(nome, v)
		}
	}
	u := fmt.Sprintf("/preparado/%d", id)
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// handlePrepareStart começa (ou reaproveita) o preparo.
func (s *Server) handlePrepareStart(w http.ResponseWriter, r *http.Request) {
	arquivo, err := s.db.FileByID(r.Context(), atoi64(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusNotFound, "arquivo não encontrado")
		return
	}
	if db.SoNaNuvem(arquivo.Localizacao) && s.preparaNoWorker(r.Context(), arquivo) {
		if err := s.sincro.PedirProcessamento(r.Context(), arquivo.ID); err != nil {
			writeError(w, statusDaSincro(err), err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, s.progressoNaNuvem(r.Context(), arquivo))
		return
	}
	if !s.transcodeAtivo() {
		writeError(w, http.StatusServiceUnavailable, "transcodificação desligada na configuração")
		return
	}

	plano, pedido := s.planoDeArquivo(r, arquivo)
	if plano.Mode == media.ModeDirect {
		writeError(w, http.StatusBadRequest, "este arquivo já toca direto, não há o que preparar")
		return
	}
	if ok, _ := s.prepararNaNuvem(r.Context(), arquivo, pedido); ok {
		if err := s.sincro.PedirProcessamento(r.Context(), arquivo.ID); err != nil {
			writeError(w, statusDaSincro(err), err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, s.progressoNaNuvem(r.Context(), arquivo))
		return
	}

	// O trabalho vive no contexto do servidor, não da requisição: o navegador
	// pode desistir, o preparo continua; `nas stop` mata o ffmpeg junto. Um
	// item da nuvem lê o original por URL, e aí o contexto é o da nuvem.
	fundo := s.fundo
	if db.SoNaNuvem(arquivo.Localizacao) {
		ctx, err := s.origemNaNuvem(&pedido, arquivo)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		fundo = ctx
	}
	progresso, err := s.preparador.Pedir(fundo, pedido)
	if err != nil {
		writeJSON(w, http.StatusInsufficientStorage, progresso)
		return
	}
	writeJSON(w, http.StatusAccepted, progresso)
}

// handlePrepareEvents transmite o progresso por SSE, no mesmo formato do scan.
func (s *Server) handlePrepareEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming não suportado")
		return
	}
	arquivo, err := s.db.FileByID(r.Context(), atoi64(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusNotFound, "arquivo não encontrado")
		return
	}
	_, pedido := s.planoDeArquivo(r, arquivo)
	daNuvem := db.SoNaNuvem(arquivo.Localizacao) && s.preparaNoWorker(r.Context(), arquivo)
	if !db.SoNaNuvem(arquivo.Localizacao) {
		daNuvem, _ = s.prepararNaNuvem(r.Context(), arquivo, pedido)
	}
	consulta := func() media.Progresso {
		if daNuvem {
			return s.progressoNaNuvem(r.Context(), arquivo)
		}
		return s.preparador.Consultar(pedido)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var ultimo string
	for {
		progresso := consulta()
		if payload, err := json.Marshal(progresso); err == nil && string(payload) != ultimo {
			ultimo = string(payload)
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
		// Terminou (bem ou mal): não há mais nada a transmitir.
		if progresso.Estado == media.EstadoPronto || progresso.Estado == media.EstadoErro {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

// handlePreparado serve o arquivo já preparado, com Range e seek normais — é o
// mesmo http.ServeContent do direct play, sobre um MP4 completo com o índice na
// frente.
func (s *Server) handlePreparado(w http.ResponseWriter, r *http.Request) {
	arquivo, err := s.db.FileByID(r.Context(), atoi64(r.PathValue("id")))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_, pedido := s.planoDeArquivo(r, arquivo)

	progresso := s.preparador.Consultar(pedido)
	if progresso.Estado != media.EstadoPronto {
		// 409: existe, mas ainda não. O player consulta o SSE e volta.
		writeError(w, http.StatusConflict, "o arquivo ainda está sendo preparado")
		return
	}

	f, err := os.Open(progresso.Arquivo)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "private, max-age=0")
	http.ServeContent(w, r, filepath.Base(progresso.Arquivo), info.ModTime(), f)
}

// respostaDaNuvem preenche o plano para um preparo feito pelo worker: o MP4
// compatível pronto, ou o progresso de espera. soNuvem diz se o original
// está só no bucket (sem worker, a única chance é tentar ele).
func (s *Server) respostaDaNuvem(ctx context.Context, resp *respostaPlayback, arquivo db.MediaFile, soNuvem bool) {
	progresso := s.progressoNaNuvem(ctx, arquivo)
	resp.Preparo = &progresso
	resp.FFmpeg, resp.Ativo = true, s.ConfigNuvem().FilaJobs != ""
	if progresso.Estado == media.EstadoPronto {
		resp.URL = resp.URLDireta + "?derivado=compat"
	} else if !resp.Ativo && soNuvem {
		resp.Motivo += " · sem processamento na nuvem, tentando o original"
		resp.Modo, resp.URL, resp.Preparo = media.ModeDirect, resp.URLDireta, nil
	}
}

// preparaNoWorker decide quem prepara um item que só existe na nuvem. O worker
// quando já preparou, quando há pedido recente para ele, ou quando algum worker
// já respondeu neste Mac; senão o próprio Mac, lendo o original do bucket. Sem
// isso, um Mac sem workers publicados deixava o player em "Preparando na
// nuvem" para sempre.
func (s *Server) preparaNoWorker(ctx context.Context, arquivo db.MediaFile) bool {
	if _, err := s.db.DerivadoDe(ctx, arquivo.ID, "compat", 0); err == nil {
		return true
	}
	if s.ConfigNuvem().FilaJobs == "" {
		return false
	}
	estado, _, quando := s.db.ProcessamentoDesde(ctx, arquivo.ID)
	if estado == "pedido" {
		return time.Since(quando) < sincro.EsperaPeloWorker
	}
	if estado == "falhou" || estado == "concluido" {
		return false // o worker não deu conta ou não gerou versão: o Mac faz
	}
	return s.sincro.WorkerVisto(ctx)
}

// origemNaNuvem assina a leitura do original para o ffmpeg do Mac. O preparo
// nasce de um contexto preso ao kill switch: cortar a nuvem mata o ffmpeg que
// lê dela.
func (s *Server) origemNaNuvem(pedido *media.Pedido, arquivo db.MediaFile) (context.Context, error) {
	arm, ctx, _, ok := s.nuvem.Vincular(s.fundo)
	if !ok {
		return nil, errors.New("modo local: a nuvem está desligada")
	}
	url, err := arm.URLDeLeitura(ctx, arquivo.NuvemKey, 6*time.Hour)
	if err != nil {
		return nil, err
	}
	pedido.Origem = url
	return ctx, nil
}

// prepararNaNuvem decide o bursting de um arquivo que está no Mac E no
// bucket. Nunca para um arquivo só local: subir GBs custa mais que o
// VideoToolbox do M4 recodificar. A decisão gruda: um pedido feito ao worker
// continua valendo até ele responder, para o player não alternar entre os
// dois preparos.
func (s *Server) prepararNaNuvem(ctx context.Context, arquivo db.MediaFile, pedido media.Pedido) (bool, string) {
	if arquivo.Localizacao != db.LocalAmbos || arquivo.NuvemKey == "" {
		return false, ""
	}
	if s.nuvem.Modo() != cloud.ModoHibrido || s.ConfigNuvem().FilaJobs == "" {
		return false, ""
	}
	if s.preparador.Consultar(pedido).Estado == media.EstadoPronto {
		return false, "" // já está pronto aqui: servir do disco é de graça
	}
	if _, err := s.db.DerivadoDe(ctx, arquivo.ID, "compat", 0); err == nil {
		return true, "já preparado"
	}
	switch estado, _ := s.db.Processamento(ctx, arquivo.ID); estado {
	case "pedido":
		return true, "pedido ao worker"
	case "concluido", "falhou":
		// O worker não gerou versão para este cliente: o Mac faz.
		return false, ""
	}
	// Sem nenhum worker ter respondido, mandar para a nuvem é apostar numa
	// fila que talvez ninguém leia: o Mac prepara, mesmo na bateria.
	if !s.sincro.WorkerVisto(ctx) {
		return false, ""
	}
	if e := s.opts.Energia.Estado(ctx); e.Ruim() {
		return true, "Mac " + e.Motivo
	}
	if s.preparador.Ocupado() {
		return true, "fila local cheia"
	}
	return false, ""
}

// progressoNaNuvem traduz o estado do processamento no worker para o mesmo
// formato do preparo local: o player não precisa saber quem prepara.
func (s *Server) progressoNaNuvem(ctx context.Context, arquivo db.MediaFile) media.Progresso {
	if _, err := s.db.DerivadoDe(ctx, arquivo.ID, "compat", 0); err == nil {
		return media.Progresso{Estado: media.EstadoPronto, Percentual: 100, Receita: "nuvem"}
	}
	estado, erro, quando := s.db.ProcessamentoDesde(ctx, arquivo.ID)
	switch estado {
	case "pedido":
		if time.Since(quando) >= sincro.EsperaPeloWorker {
			return media.Progresso{Estado: media.EstadoErro, Receita: "nuvem",
				Erro: "nenhum worker da nuvem respondeu; abra de novo para o Mac preparar (ou publique a imagem com make publicar-imagem)"}
		}
		return media.Progresso{Estado: media.EstadoTrabalhando, Receita: "nuvem", Total: arquivo.Duration}
	case "falhou":
		return media.Progresso{Estado: media.EstadoErro, Receita: "nuvem", Erro: "o worker da nuvem falhou: " + erro}
	case "concluido":
		// Processado, mas sem compat: o worker achou que o original toca.
		return media.Progresso{Estado: media.EstadoErro, Receita: "nuvem",
			Erro: "a nuvem processou o arquivo e não gerou versão compatível"}
	}
	return media.Progresso{Estado: media.EstadoAusente, Receita: "nuvem"}
}

func (s *Server) transcodeAtivo() bool {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg.Transcode
}
