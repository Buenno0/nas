package sincro

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
)

const (
	// O S3 aceita no máximo 10 000 partes, de 5 MiB a 5 GiB cada. 16 MiB dá
	// arquivos de até ~156 GiB com a parte mínima; acima disso a parte cresce.
	parteMinima = 16 << 20
	maxPartes   = 9000
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

// Interpretar desfaz ChaveDoUpload: "bibliotecas/3/ab12cd/Série/ep.mkv" vira
// biblioteca 3 e caminho relativo "Série/ep.mkv".
func Interpretar(key string) (libID int64, rel string, ok bool) {
	partes := strings.SplitN(key, "/", 4)
	if len(partes) != 4 || partes[0] != "bibliotecas" || partes[3] == "" {
		return 0, "", false
	}
	id, err := strconv.ParseInt(partes[1], 10, 64)
	if err != nil {
		return 0, "", false
	}
	rel, err = relSeguro(partes[3])
	if err != nil {
		return 0, "", false
	}
	return id, rel, true
}
