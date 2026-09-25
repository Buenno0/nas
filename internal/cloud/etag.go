package cloud

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// ETagLocal recalcula, sobre um arquivo do disco, o ETag que o S3 daria a
// ele. É assim que "liberar espaço" prova que a cópia na nuvem é idêntica
// antes de apagar a local, sem baixar nada.
//
// PUT único: md5 do arquivo. Multipart: md5 da concatenação dos md5 de cada
// parte, seguido de "-N".
func ETagLocal(caminho string, tamanhoParte int64) (string, error) {
	f, err := os.Open(caminho)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if tamanhoParte <= 0 {
		h := md5.New()
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}

	var (
		juntos []byte
		n      int
	)
	for {
		h := md5.New()
		lidos, err := io.CopyN(h, f, tamanhoParte)
		if lidos > 0 {
			juntos = append(juntos, h.Sum(nil)...)
			n++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	soma := md5.Sum(juntos)
	return fmt.Sprintf("%s-%d", hex.EncodeToString(soma[:]), n), nil
}

// MesmoConteudo compara um ETag local com o do bucket (que vem entre aspas).
func MesmoConteudo(local, remoto string) bool {
	return local != "" && local == strings.Trim(remoto, `"`)
}
