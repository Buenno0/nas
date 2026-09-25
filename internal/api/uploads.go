package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"nas/internal/auth"
	"nas/internal/cloud"
	"nas/internal/db"
	"nas/internal/scan"
)

const (
	// O S3 aceita no máximo 10 000 partes, de 5 MiB a 5 GiB cada. 16 MiB dá
	// arquivos de até ~156 GiB com a parte mínima; acima disso a parte cresce.
	parteMinima      = 16 << 20
	maxPartes        = 9000
	maxTamanho       = 5 << 40
	ttlDaParte       = time.Hour
	ttlDeLeitura     = time.Hour
	maxURLsPorPedido = 100
)

// TamanhoDaParte escolhe a parte para caber no limite de partes do S3.
func TamanhoDaParte(total int64) int64 {
	p := int64(parteMinima)
	if n := (total + maxPartes - 1) / maxPartes; n > p {
		// Arredonda para MiB: o navegador fatia melhor em números redondos.
		p = (n + (1 << 20) - 1) &^ ((1 << 20) - 1)
	}
	return p
}

// ChaveDoUpload monta a chave no bucket. O sufixo aleatório impede que dois
// envios com o mesmo nome se sobrescrevam.
func ChaveDoUpload(libraryID int64, rel string) (string, error) {
	limpo, err := relSeguro(rel)
	if err != nil {
		return "", err
	}
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("bibliotecas/%d/%s/%s", libraryID, hex.EncodeToString(b[:]), limpo), nil
}

// relSeguro aceita "Série/Temporada 1/ep.mkv", recusa quem tenta subir de
// diretório ou mandar caminho absoluto.
func relSeguro(rel string) (string, error) {
	rel = strings.ReplaceAll(strings.TrimSpace(rel), `\`, "/")
	limpo := path.Clean("/" + rel)[1:]
	if limpo == "" || limpo == "." || strings.Contains(rel, "..") {
		return "", errors.New("nome de arquivo inválido")
	}
	return limpo, nil
}

// nuvemOu503 devolve o bucket, ou responde que o modo é local.
func (s *Server) nuvemOu503(w http.ResponseWriter) (cloud.Armazenamento, context.Context, bool) {
	arm, ctx, ok := s.nuvem.Hibrido()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "modo local: a nuvem está desligada")
		return nil, nil, false
	}
	return arm, ctx, true
}

func (s *Server) handleUploads(w http.ResponseWriter, r *http.Request) {
	ups, err := s.db.UploadsPendentes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ups == nil {
		ups = []db.Upload{}
	}
	writeJSON(w, http.StatusOK, ups)
}

func (s *Server) handleCriarUpload(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LibraryID   int64  `json:"library_id"`
		Nome        string `json:"nome"`
		Tamanho     int64  `json:"tamanho"`
		ContentType string `json:"content_type"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if body.Tamanho <= 0 || body.Tamanho > maxTamanho {
		writeError(w, http.StatusBadRequest, "tamanho inválido")
		return
	}
	if _, ok := scan.TipoPorExtensao(body.Nome); !ok {
		writeError(w, http.StatusBadRequest, "formato que o Ozymandias não indexa")
		return
	}
	lib, err := s.db.Library(r.Context(), body.LibraryID)
	if err != nil {
		writeError(w, http.StatusNotFound, "biblioteca não encontrada")
		return
	}
	arm, nctx, ok := s.nuvemOu503(w)
	if !ok {
		return
	}
	key, err := ChaveDoUpload(lib.ID, body.Nome)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := juntos(r.Context(), nctx)
	defer cancel()
	uploadID, err := arm.IniciarEnvio(ctx, key, body.ContentType)
	if err != nil {
		writeError(w, http.StatusBadGateway, "o bucket recusou o envio: "+err.Error())
		return
	}

	u := db.Upload{
		LibraryID: lib.ID, Key: key, UploadID: uploadID, Nome: body.Nome,
		Tamanho: body.Tamanho, ParteTamanho: TamanhoDaParte(body.Tamanho),
		ContentType: body.ContentType, CreatedAt: time.Now().Unix(),
	}
	if user, ok := auth.UserFrom(r.Context()); ok {
		u.UserID = &user.ID
	}
	id, err := s.db.CriaUpload(r.Context(), u)
	if err != nil {
		_ = arm.AbortarEnvio(ctx, key, uploadID)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	u.ID = id
	u.Estado = "enviando"
	writeJSON(w, http.StatusCreated, map[string]any{
		"upload": u,
		"partes": (u.Tamanho + u.ParteTamanho - 1) / u.ParteTamanho,
	})
}

// uploadAberto carrega o upload do caminho e confere que ainda aceita partes.
func (s *Server) uploadAberto(w http.ResponseWriter, r *http.Request) (db.Upload, bool) {
	u, err := s.db.UploadPorID(r.Context(), atoi64(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusNotFound, "upload não encontrado")
		return db.Upload{}, false
	}
	if u.Estado != "enviando" {
		writeError(w, http.StatusConflict, "upload já "+u.Estado)
		return db.Upload{}, false
	}
	return u, true
}

// handleURLsDoUpload assina as partes pedidas. URLs curtas (1 h) e emitidas
// aos poucos: se o kill switch for acionado, o que ainda não foi pedido não
// sai mais do Mac.
func (s *Server) handleURLsDoUpload(w http.ResponseWriter, r *http.Request) {
	u, ok := s.uploadAberto(w, r)
	if !ok {
		return
	}
	var body struct {
		Partes []int32 `json:"partes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if len(body.Partes) == 0 || len(body.Partes) > maxURLsPorPedido {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("peça de 1 a %d partes", maxURLsPorPedido))
		return
	}
	total := int32((u.Tamanho + u.ParteTamanho - 1) / u.ParteTamanho)
	arm, nctx, ok := s.nuvemOu503(w)
	if !ok {
		return
	}
	ctx, cancel := juntos(r.Context(), nctx)
	defer cancel()

	urls := make(map[int32]string, len(body.Partes))
	for _, n := range body.Partes {
		if n < 1 || n > total {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("parte %d fora de 1..%d", n, total))
			return
		}
		url, err := arm.URLDaParte(ctx, u.Key, u.UploadID, n, ttlDaParte)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		urls[n] = url
	}
	writeJSON(w, http.StatusOK, map[string]any{"urls": urls})
}

// handlePartesDoUpload diz o que o bucket já recebeu: é como o navegador
// retoma depois do kill switch sem reenviar tudo.
func (s *Server) handlePartesDoUpload(w http.ResponseWriter, r *http.Request) {
	u, ok := s.uploadAberto(w, r)
	if !ok {
		return
	}
	arm, nctx, ok := s.nuvemOu503(w)
	if !ok {
		return
	}
	ctx, cancel := juntos(r.Context(), nctx)
	defer cancel()
	partes, err := arm.PartesEnviadas(ctx, u.Key, u.UploadID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if partes == nil {
		partes = []cloud.Parte{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"upload": u, "enviadas": partes})
}

func (s *Server) handleConcluirUpload(w http.ResponseWriter, r *http.Request) {
	u, ok := s.uploadAberto(w, r)
	if !ok {
		return
	}
	var body struct {
		Partes []cloud.Parte `json:"partes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	arm, nctx, ok := s.nuvemOu503(w)
	if !ok {
		return
	}
	ctx, cancel := juntos(r.Context(), nctx)
	defer cancel()

	// Sem a lista do cliente (ou com ela incompleta), vale o que o bucket diz.
	partes := body.Partes
	if len(partes) == 0 {
		var err error
		if partes, err = arm.PartesEnviadas(ctx, u.Key, u.UploadID); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
	}
	sort.Slice(partes, func(i, j int) bool { return partes[i].Numero < partes[j].Numero })
	if err := arm.ConcluirEnvio(ctx, u.Key, u.UploadID, partes); err != nil {
		writeError(w, http.StatusBadGateway, "o bucket não fechou o envio: "+err.Error())
		return
	}
	tamanho, err := arm.Tamanho(ctx, u.Key)
	if err != nil || tamanho != u.Tamanho {
		writeError(w, http.StatusBadGateway,
			fmt.Sprintf("o bucket tem %d bytes, esperados %d", tamanho, u.Tamanho))
		return
	}
	if err := s.db.EstadoDoUpload(r.Context(), u.ID, "concluido"); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	fileID, err := s.indexarDaNuvem(ctx, arm, u)
	if err != nil {
		// O arquivo está salvo no bucket; só o índice falhou. Não é perda.
		log.Printf("indexando %s: %v", u.Key, err)
		writeError(w, http.StatusInternalServerError, "enviado, mas não foi possível indexar: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"file_id": fileID})
}

// indexarDaNuvem lê o cabeçalho do arquivo pela URL assinada (ffprobe por
// Range, sem baixar tudo) e coloca o item no catálogo como "nuvem".
func (s *Server) indexarDaNuvem(ctx context.Context, arm cloud.Armazenamento, u db.Upload) (int64, error) {
	lib, err := s.db.Library(ctx, u.LibraryID)
	if err != nil {
		return 0, err
	}
	leitura, err := arm.URLDeLeitura(ctx, u.Key, ttlDeLeitura)
	if err != nil {
		return 0, err
	}
	return scan.New(s.db).IndexarNuvem(ctx, lib, u.Key, u.Nome, u.Tamanho, leitura)
}

func (s *Server) handleAbortarUpload(w http.ResponseWriter, r *http.Request) {
	u, ok := s.uploadAberto(w, r)
	if !ok {
		return
	}
	// Abortar no bucket precisa do híbrido; sem ele, a linha fica marcada e a
	// regra de lifecycle do bucket limpa as partes órfãs.
	if arm, nctx, ok := s.nuvem.Hibrido(); ok {
		ctx, cancel := juntos(r.Context(), nctx)
		defer cancel()
		if err := arm.AbortarEnvio(ctx, u.Key, u.UploadID); err != nil {
			log.Printf("abortando %s no bucket: %v", u.Key, err)
		}
	}
	if err := s.db.EstadoDoUpload(r.Context(), u.ID, "abortado"); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// juntos devolve um contexto que morre com a requisição OU com o kill switch.
func juntos(req, nuvem context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(req)
	parar := context.AfterFunc(nuvem, cancel)
	return ctx, func() { parar(); cancel() }
}
