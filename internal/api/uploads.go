package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"nas/internal/auth"
	"nas/internal/cloud"
	"nas/internal/db"
	"nas/internal/scan"
	"nas/internal/sincro"
)

const (
	ttlDaParte       = time.Hour
	ttlDeLeitura     = time.Hour
	maxURLsPorPedido = 100
	maxTamanho       = 5 << 40
)

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
	key, err := sincro.ChaveDoUpload(lib.ID, body.Nome)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := juntos(r.Context(), nctx)
	defer cancel()

	if u, ok := s.reaproveitaUpload(ctx, arm, lib.ID, body.Nome, body.Tamanho); ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"upload": u,
			"partes": (u.Tamanho + u.ParteTamanho - 1) / u.ParteTamanho,
		})
		return
	}
	uploadID, err := arm.IniciarEnvio(ctx, key, body.ContentType)
	if err != nil {
		writeError(w, http.StatusBadGateway, "o bucket recusou o envio: "+err.Error())
		return
	}

	u := db.Upload{
		LibraryID: lib.ID, Key: key, UploadID: uploadID, Nome: body.Nome,
		Tamanho: body.Tamanho, ParteTamanho: sincro.TamanhoDaParte(body.Tamanho),
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
	s.db.Anota(r.Context(), "envio.inicio", u.ID, 0, u.Nome, map[string]any{
		"origem": "navegador", "tamanho": u.Tamanho, "parte_tamanho": u.ParteTamanho, "key": u.Key})
	writeJSON(w, http.StatusCreated, map[string]any{
		"upload": u,
		"partes": (u.Tamanho + u.ParteTamanho - 1) / u.ParteTamanho,
	})
}

// reaproveitaUpload devolve um envio aberto do mesmo arquivo, se o bucket
// ainda o conhece. O navegador então retoma pelas partes que já estão lá. Um
// envio que o bucket esqueceu (abortado, limpo pelo lifecycle) é fechado aqui.
func (s *Server) reaproveitaUpload(ctx context.Context, arm cloud.Armazenamento, libID int64, nome string, tamanho int64) (db.Upload, bool) {
	u, err := s.db.UploadAbertoDoNavegador(ctx, libID, nome, tamanho)
	if err != nil {
		return db.Upload{}, false
	}
	if _, err := arm.PartesEnviadas(ctx, u.Key, u.UploadID); err != nil {
		if errors.Is(err, cloud.ErrNaoExiste) {
			_ = s.db.EstadoDoUpload(ctx, u.ID, "abortado")
			s.db.Anota(ctx, "envio.abortado", u.ID, 0, u.Nome, map[string]any{"motivo": "o bucket não conhece mais este envio"})
		}
		return db.Upload{}, false
	}
	s.db.Anota(ctx, "envio.retomada", u.ID, 0, u.Nome, map[string]any{"motivo": "mesmo arquivo solto de novo"})
	return u, true
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
	s.db.Anota(r.Context(), "envio.urls", u.ID, 0, u.Nome, map[string]any{"partes": body.Partes})
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
		s.db.Anota(r.Context(), "envio.erro", u.ID, 0, u.Nome, map[string]any{"erro": err.Error()})
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
	s.db.Anota(r.Context(), "envio.concluido", u.ID, fileID, u.Nome, map[string]any{
		"origem": "navegador", "partes": len(partes), "tamanho": tamanho})
	if err != nil {
		// O arquivo está salvo no bucket; só o índice falhou. Não é perda.
		log.Printf("indexando %s: %v", u.Key, err)
		writeError(w, http.StatusInternalServerError, "enviado, mas não foi possível indexar: "+err.Error())
		return
	}
	s.capaDoUpload(fileID)
	writeJSON(w, http.StatusOK, map[string]any{"file_id": fileID})
}

// capaDoUpload busca os metadados do título recém-enviado em segundo plano: a
// resposta não espera o TMDB, e a página do título pega a capa no próximo
// refetch.
func (s *Server) capaDoUpload(fileID int64) {
	go func() {
		ctx, cancel := context.WithTimeout(s.fundo, time.Minute)
		defer cancel()
		f, err := s.db.FileByID(ctx, fileID)
		if err != nil || f.TitleID == nil {
			return
		}
		enricher, err := s.enricher()
		if err == nil {
			err = enricher.EnrichUpload(ctx, *f.TitleID)
		}
		if err != nil {
			log.Printf("capa do upload %d: %v", fileID, err)
		}
	}()
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
	s.db.Anota(r.Context(), "envio.abortado", u.ID, 0, u.Nome, nil)
	if err := s.db.EstadoDoUpload(r.Context(), u.ID, "abortado"); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDiarioDoUpload recebe do navegador o que só ele vê: cada parte que
// o S3 aceitou (com o tempo que levou), pausas e erros. É só o diário.
func (s *Server) handleDiarioDoUpload(w http.ResponseWriter, r *http.Request) {
	u, err := s.db.UploadPorID(r.Context(), atoi64(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusNotFound, "upload não encontrado")
		return
	}
	var body struct {
		Tipo    string `json:"tipo"`
		N       int32  `json:"n,omitempty"`
		Tamanho int64  `json:"tamanho,omitempty"`
		Ms      int64  `json:"ms,omitempty"`
		ETag    string `json:"etag,omitempty"`
		Erro    string `json:"erro,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	dados := map[string]any{}
	switch body.Tipo {
	case "parte":
		dados = map[string]any{"n": body.N, "tamanho": body.Tamanho, "ms": body.Ms, "etag": strings.Trim(body.ETag, `"`)}
	case "pausa":
		dados["motivo"] = "kill switch"
	case "retomada":
	case "erro":
		dados["erro"] = body.Erro
	default:
		writeError(w, http.StatusBadRequest, "tipo deve ser parte, pausa, retomada ou erro")
		return
	}
	s.db.Anota(r.Context(), "envio."+body.Tipo, u.ID, 0, u.Nome, dados)
	w.WriteHeader(http.StatusNoContent)
}

// juntos devolve um contexto que morre com a requisição OU com o kill switch.
func juntos(req, nuvem context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(req)
	parar := context.AfterFunc(nuvem, cancel)
	return ctx, func() { parar(); cancel() }
}
