package api

import "net/http"

// handleArmazenamento alimenta a tela Armazenamento: o que mora no Mac, o que
// mora na nuvem e o disco. Só lê o banco e o disco — nenhuma chamada à AWS;
// o tamanho do bucket e as classes vêm de /api/tecnico (em cache).
func (s *Server) handleArmazenamento(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locais, err := s.db.UsoPorLocalizacao(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	bibs, err := s.db.UsoPorBiblioteca(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	comDerivados, derivados, err := s.db.ContaDerivados(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{
		"por_localizacao": locais,
		"por_biblioteca":  bibs,
		"derivados":       map[string]int64{"arquivos": comDerivados, "total": derivados},
	}
	// A instância cloud não tem disco de mídia: o card do disco some lá.
	if !s.opts.NaNuvem {
		livre, total := s.preparador.Disco()
		resp["disco"] = map[string]int64{"livre": livre, "total": total, "reserva": s.preparador.Reserva()}
	}
	writeJSON(w, http.StatusOK, resp)
}
