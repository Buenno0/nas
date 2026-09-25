package api

import (
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

	// Item só da nuvem: toca direto da CDN/bucket ou não toca. O preparo
	// (transcodificação) de itens da nuvem é trabalho dos workers do V3.
	if db.SoNaNuvem(arquivo.Localizacao) {
		if s.nuvem.Modo() != cloud.ModoHibrido {
			resp.Indisponivel = true
			resp.Motivo = "na nuvem, indisponível no modo local"
			writeJSON(w, http.StatusOK, resp)
			return
		}
		if plano.Mode != media.ModeDirect {
			resp.Motivo = "na nuvem: tentando o original (preparo de itens da nuvem ainda não existe)"
		}
		resp.Modo = media.ModeDirect
		resp.URL = resp.URLDireta
		writeJSON(w, http.StatusOK, resp)
		return
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
	if db.SoNaNuvem(arquivo.Localizacao) {
		writeError(w, http.StatusConflict, "itens só da nuvem ainda não são preparados")
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

	// O trabalho vive no contexto do servidor, não da requisição: o navegador
	// pode desistir, o preparo continua; `nas stop` mata o ffmpeg junto.
	progresso, err := s.preparador.Pedir(s.fundo, pedido)
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

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var ultimo string
	for {
		progresso := s.preparador.Consultar(pedido)
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

func (s *Server) transcodeAtivo() bool {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg.Transcode
}
