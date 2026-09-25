package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"nas/internal/config"
	"nas/internal/db"
	"nas/internal/media"
	"nas/internal/web"
)

// Tipos MIME que o Go não conhece ou erra, e que o <video> precisa acertar.
var mimeByExt = map[string]string{
	".mkv":  "video/x-matroska",
	".m4v":  "video/mp4",
	".mov":  "video/quicktime",
	".avi":  "video/x-msvideo",
	".ts":   "video/mp2t",
	".m2ts": "video/mp2t",
	".webm": "video/webm",
	".mp4":  "video/mp4",
	".mp3":  "audio/mpeg",
	".m4a":  "audio/mp4",
	".flac": "audio/flac",
	".ogg":  "audio/ogg",
	".opus": "audio/ogg",
	".wav":  "audio/wav",
	".aac":  "audio/aac",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
	".gif":  "image/gif",
	".heic": "image/heic",
	".avif": "image/avif",
}

// handleStream entrega o arquivo cru. http.ServeContent cuida de Range, seek,
// If-Modified-Since e 206 — não há paginação manual de bytes aqui.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	file, err := s.db.FileByID(r.Context(), atoi64(r.PathValue("id")))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Item só da nuvem, ou o derivado de um item que também está no Mac (o
	// MP4 que o worker preparou no bursting).
	if db.SoNaNuvem(file.Localizacao) || r.URL.Query().Get("derivado") != "" {
		s.streamDaNuvem(w, r, file)
		return
	}
	if s.opts.NaNuvem {
		if web.QuerTela(r) {
			web.ServeTela(w, r, "503-no-mac", http.StatusServiceUnavailable)
			return
		}
		writeError(w, http.StatusServiceUnavailable, "no Mac, indisponível na nuvem")
		return
	}

	f, err := os.Open(file.Path)
	if err != nil {
		// O índice pode estar à frente do disco (arquivo movido, HD externo fora).
		writeError(w, http.StatusNotFound, "arquivo indisponível no disco")
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "não foi possível ler o arquivo")
		return
	}

	if ct, ok := mimeByExt[strings.ToLower(file.Ext)]; ok {
		w.Header().Set("Content-Type", ct)
	}
	if r.URL.Query().Get("download") != "" {
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename*=UTF-8''%s", escapeFilename(filepath.Base(file.Path))))
	}
	// O conteúdo é privado e o cache do navegador é por sessão.
	w.Header().Set("Cache-Control", "private, max-age=0")

	http.ServeContent(w, r, filepath.Base(file.Path), info.ModTime(), f)
}

// streamDaNuvem manda o cliente ler direto do bucket: os bytes não passam pelo
// Mac. No modo local o item existe no catálogo, mas não toca.
func (s *Server) streamDaNuvem(w http.ResponseWriter, r *http.Request, file db.MediaFile) {
	arm, _, ok := s.nuvem.Hibrido()
	if !ok {
		if web.QuerTela(r) {
			web.ServeTela(w, r, "503", http.StatusServiceUnavailable)
			return
		}
		writeError(w, http.StatusServiceUnavailable, "na nuvem, indisponível no modo local")
		return
	}
	key := file.NuvemKey
	if tipo := r.URL.Query().Get("derivado"); tipo != "" {
		d, err := s.db.DerivadoDe(r.Context(), file.ID, tipo, atoiDefault(r.URL.Query().Get("i"), 0))
		if err != nil {
			writeError(w, http.StatusNotFound, "derivado não existe")
			return
		}
		key = d
	}
	url, err := arm.URLDeLeitura(r.Context(), key, ttlDeLeitura)
	if err != nil {
		if web.QuerTela(r) {
			web.ServeTela(w, r, "502", http.StatusBadGateway)
			return
		}
		writeError(w, http.StatusBadGateway, "não foi possível assinar a leitura: "+err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, url, http.StatusFound)
}

// derivadosDeImagem diz, em ordem de preferência, que imagem do worker serve
// de fonte para a miniatura pedida.
func derivadosDeImagem(t db.MediaType, largura int) []string {
	switch t {
	case db.TypeVideo:
		return []string{"frame"}
	case db.TypePhoto:
		if largura <= 320 {
			return []string{"thumb320", "thumb800"}
		}
		return []string{"thumb800"}
	}
	return nil
}

// Larguras permitidas para as miniaturas. Uma lista fechada evita que alguém
// peça mil tamanhos diferentes e encha o disco de JPEG.
var thumbWidths = map[int]bool{320: true, 800: true, 1600: true}

// handleFileThumb gera (e cacheia) a miniatura de uma foto ou de um vídeo.
// É o que faz a galeria carregar rápido e o que resolve HEIC, formato que o
// Chrome não abre — a conversão para JPEG acontece aqui.
func (s *Server) handleFileThumb(w http.ResponseWriter, r *http.Request) {
	file, err := s.db.FileByID(r.Context(), atoi64(r.PathValue("id")))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	width := atoiDefault(r.URL.Query().Get("w"), 320)
	if !thumbWidths[width] {
		width = 320
	}

	dir, err := config.ThumbDir()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Item da nuvem: a chave do cache é o caminho estável, e a leitura sai de
	// uma URL assinada. No modo local, só o que já estiver em cache aparece.
	ctx, origem := r.Context(), file.Path
	if s.opts.NaNuvem && !db.SoNaNuvem(file.Localizacao) {
		origem = "" // só no Mac: aqui, só o que já estiver em cache
	} else if db.SoNaNuvem(file.Localizacao) {
		arm, vida, ok := s.nuvem.Hibrido()
		if ok {
			// A imagem pronta do worker é um JPEG de KB; o original pode ser
			// um vídeo de GB. Com ela, a miniatura não lê o vídeo pela rede.
			key := file.NuvemKey
			for _, tipo := range derivadosDeImagem(file.Type, width) {
				if d, err := s.db.DerivadoDe(ctx, file.ID, tipo, 0); err == nil {
					key = d
					break
				}
			}
			origem, err = arm.URLDeLeitura(ctx, key, ttlDeLeitura)
		}
		if !ok || err != nil {
			origem = ""
		} else {
			ctx = media.ComVida(ctx, vida)
		}
	}

	var name string
	switch file.Type {
	case db.TypePhoto:
		name, err = media.ImageThumbDe(ctx, file.Path, origem, file.MTime, dir, width)
	case db.TypeVideo:
		name, err = media.VideoFrameDe(ctx, file.Path, origem, file.MTime, file.Duration, dir)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		if origem == "" {
			writeError(w, http.StatusServiceUnavailable, "na nuvem, indisponível no modo local")
			return
		}
		if errors.Is(err, media.ErrNoFFmpeg) {
			writeError(w, http.StatusServiceUnavailable, "ffmpeg não instalado: sem miniaturas")
			return
		}
		writeError(w, http.StatusInternalServerError, "não foi possível gerar a miniatura")
		return
	}

	f, err := os.Open(filepath.Join(dir, name))
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
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=604800, immutable")
	http.ServeContent(w, r, name, info.ModTime(), f)
}

// handleImage serve pôsteres e thumbnails do cache em ~/.nas/cache.
func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	name := r.PathValue("name")

	// O nome vem do banco, mas a URL vem do cliente: nada de subir diretório.
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}

	var dir string
	var err error
	switch kind {
	case "posters":
		dir, err = config.PosterDir()
	case "thumbs":
		dir, err = config.ThumbDir()
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	path := filepath.Join(dir, name)
	f, err := os.Open(path)
	if err != nil && kind == "posters" && s.opts.NaNuvem && s.sincro.PosterDaNuvem(r.Context(), name) {
		f, err = os.Open(path)
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	// Imagens de cache são imutáveis: o nome muda quando o conteúdo muda.
	w.Header().Set("Cache-Control", "private, max-age=604800, immutable")
	http.ServeContent(w, r, name, info.ModTime(), f)
}

// escapeFilename deixa o nome seguro para o cabeçalho Content-Disposition.
func escapeFilename(name string) string {
	var b strings.Builder
	for _, c := range []byte(name) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '.' || c == '-' || c == '_' {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}
