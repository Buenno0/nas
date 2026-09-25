package cloud

import (
	"context"
	"net/http"

	"nas/internal/config"
)

// semSuporte é o conector de um binário sem adapter: o modo local continua
// inteiro, e pedir o híbrido dá um erro claro em vez de pânico.
func semSuporte(context.Context, config.Nuvem, *http.Client) (Armazenamento, error) {
	return nil, ErrSemSuporte
}
