package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"nas/internal/db"
	"nas/internal/media"
)

// faixaJSON é uma faixa como o player a vê. O caminho em disco da legenda
// externa não vai junto: é do dono da máquina, como o das bibliotecas.
type faixaJSON struct {
	Indice       int    `json:"idx"`
	Codec        string `json:"codec,omitempty"`
	Lang         string `json:"lang,omitempty"`
	Rotulo       string `json:"rotulo"`
	Canais       int    `json:"canais,omitempty"`
	Padrao       bool   `json:"padrao,omitempty"`
	Forcada      bool   `json:"forcada,omitempty"`
	Externa      bool   `json:"externa,omitempty"`
	URL          string `json:"url,omitempty"`          // só legenda
	Indisponivel string `json:"indisponivel,omitempty"` // motivo, quando não dá para usar
}

type faixasResposta struct {
	Audio    []faixaJSON `json:"audio"`
	Legendas []faixaJSON `json:"legendas"`
}

// idiomas traduz os códigos que aparecem nos arquivos. ISO 639-2, ISO 639-1 e
// as variantes que os grupos de legenda usam na prática.
var idiomas = map[string]string{
	"por": "Português", "pt": "Português", "pt-br": "Português (BR)", "ptbr": "Português (BR)",
	"pt-pt": "Português (PT)", "bra": "Português (BR)",
	"eng": "Inglês", "en": "Inglês", "en-us": "Inglês",
	"spa": "Espanhol", "es": "Espanhol", "esp": "Espanhol",
	"fra": "Francês", "fre": "Francês", "fr": "Francês",
	"deu": "Alemão", "ger": "Alemão", "de": "Alemão",
	"ita": "Italiano", "it": "Italiano",
	"jpn": "Japonês", "ja": "Japonês",
	"kor": "Coreano", "ko": "Coreano",
	"zho": "Chinês", "chi": "Chinês", "zh": "Chinês",
	"rus": "Russo", "ru": "Russo",
	"und": "", "": "",
}

func nomeDoIdioma(codigo string) string {
	return idiomas[strings.ToLower(strings.TrimSpace(codigo))]
}

// rotuloDaFaixa monta o texto do menu. A ordem de preferência é: o título que
// veio no arquivo (quem produziu escreveu "Dublado" por algum motivo), depois o
// idioma traduzido, e só então o código cru — que é feio mas é honesto.
func rotuloDaFaixa(st db.Stream) string {
	partes := []string{}

	if st.Title != "" {
		partes = append(partes, st.Title)
	} else if nome := nomeDoIdioma(st.Lang); nome != "" {
		partes = append(partes, nome)
	} else if st.Lang != "" {
		partes = append(partes, st.Lang)
	}

	if len(partes) == 0 {
		if st.Kind == db.StreamAudio {
			partes = append(partes, fmt.Sprintf("Faixa %d", st.Index))
		} else {
			partes = append(partes, "Legenda")
		}
	}

	if st.Kind == db.StreamAudio {
		if desc := descricaoDeCanais(st.Channels); desc != "" {
			partes = append(partes, desc)
		}
	}
	if st.Forced {
		partes = append(partes, "forçada")
	}
	return strings.Join(partes, " · ")
}

// descricaoDeCanais traduz a contagem para o que as pessoas realmente dizem.
func descricaoDeCanais(n int) string {
	switch n {
	case 0:
		return ""
	case 1:
		return "mono"
	case 2:
		return "estéreo"
	case 6:
		return "5.1"
	case 8:
		return "7.1"
	default:
		return fmt.Sprintf("%d canais", n)
	}
}

// handleFaixas lista áudios e legendas de um arquivo.
func (s *Server) handleFaixas(w http.ResponseWriter, r *http.Request) {
	id := atoi64(r.PathValue("id"))
	if _, err := s.db.FileByID(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "arquivo não encontrado")
		return
	}

	streams, err := s.db.Streams(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := faixasResposta{Audio: []faixaJSON{}, Legendas: []faixaJSON{}}
	for _, st := range streams {
		f := faixaJSON{
			Indice:  st.Index,
			Codec:   st.Codec,
			Lang:    st.Lang,
			Rotulo:  rotuloDaFaixa(st),
			Canais:  st.Channels,
			Padrao:  st.Default,
			Forcada: st.Forced,
			Externa: st.ExtPath != "",
		}
		switch st.Kind {
		case db.StreamAudio:
			resp.Audio = append(resp.Audio, f)
		case db.StreamSubtitle:
			// Legenda de imagem aparece na lista, mas explicando por que não
			// dá — esconder faria parecer que o arquivo não tem legenda.
			if media.EhLegendaDeImagem(st.Codec) {
				f.Indisponivel = media.ErrLegendaDeImagem{Codec: st.Codec}.Error()
			} else {
				f.URL = fmt.Sprintf("/api/files/%d/legenda/%d.vtt", id, st.Index)
			}
			resp.Legendas = append(resp.Legendas, f)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleLegenda entrega a legenda já convertida para WebVTT.
func (s *Server) handleLegenda(w http.ResponseWriter, r *http.Request) {
	id := atoi64(r.PathValue("id"))
	arquivo, err := s.db.FileByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// O nome vem como "3.vtt": o sufixo existe só para o navegador reconhecer
	// o tipo pela URL.
	idx, err := strconv.Atoi(strings.TrimSuffix(r.PathValue("idx"), ".vtt"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	faixa, err := s.db.StreamByIndex(r.Context(), id, idx)
	if err != nil || faixa.Kind != db.StreamSubtitle {
		http.NotFound(w, r)
		return
	}

	pedido := media.PedidoLegenda{
		Origem: arquivo.Path,
		MTime:  arquivo.MTime,
		Codec:  faixa.Codec,
		Indice: faixa.Index,
	}
	// Legenda em arquivo separado: a origem é o próprio .srt, e não há stream
	// a mapear dentro dele.
	if faixa.ExtPath != "" {
		pedido.Origem = faixa.ExtPath
		pedido.Indice = -1
		if info, err := os.Stat(faixa.ExtPath); err == nil {
			pedido.MTime = info.ModTime().Unix()
		}
	}

	caminho, err := s.preparador.LegendaVTT(r.Context(), pedido)
	if err != nil {
		var imagem media.ErrLegendaDeImagem
		if errors.As(err, &imagem) {
			writeError(w, http.StatusUnsupportedMediaType, imagem.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	f, err := os.Open(caminho)
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

	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=604800")
	http.ServeContent(w, r, "legenda.vtt", info.ModTime(), f)
}
