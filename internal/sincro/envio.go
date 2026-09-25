// Package sincro move arquivos entre o Mac e o bucket: envia, fixa ("disponível
// offline"), libera espaço, espelha bibliotecas e reconcilia o catálogo ao
// entrar no híbrido. Tudo aqui obedece ao kill switch: cada operação roda num
// contexto vinculado à chave e para no instante em que o modo vira local,
// deixando o trabalho retomável.
package sincro

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"nas/internal/cloud"
	"nas/internal/db"
)

// EnviarArquivo manda as partes que o bucket ainda não tem e devolve a lista
// completa, em ordem. progresso recebe os bytes já no bucket.
func EnviarArquivo(ctx context.Context, arm cloud.Armazenamento, u db.Upload, origem string, progresso func(feitos int64)) ([]cloud.Parte, error) {
	feitas, err := arm.PartesEnviadas(ctx, u.Key, u.UploadID)
	if err != nil {
		return nil, fmt.Errorf("consultando partes: %w", err)
	}
	f, err := os.Open(origem)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || info.Size() != u.Tamanho {
		return nil, errors.New("o arquivo mudou de tamanho desde o início do envio")
	}

	tamanhoDa := func(n int32) int64 {
		off := int64(n-1) * u.ParteTamanho
		return min(u.ParteTamanho, u.Tamanho-off)
	}
	tem := map[int32]cloud.Parte{}
	var feitos int64
	for _, p := range feitas {
		tem[p.Numero] = p
		feitos += tamanhoDa(p.Numero)
	}
	if progresso != nil {
		progresso(feitos)
	}

	total := int32((u.Tamanho + u.ParteTamanho - 1) / u.ParteTamanho)
	for n := int32(1); n <= total; n++ {
		if _, ok := tem[n]; ok {
			continue
		}
		off := int64(n-1) * u.ParteTamanho
		tam := tamanhoDa(n)
		etag, err := arm.EnviarParte(ctx, u.Key, u.UploadID, n, io.NewSectionReader(f, off, tam), tam)
		if err != nil {
			return nil, fmt.Errorf("parte %d: %w", n, err)
		}
		tem[n] = cloud.Parte{Numero: n, ETag: etag}
		feitos += tam
		if progresso != nil {
			progresso(feitos)
		}
	}

	partes := make([]cloud.Parte, 0, len(tem))
	for _, p := range tem {
		partes = append(partes, p)
	}
	sort.Slice(partes, func(i, j int) bool { return partes[i].Numero < partes[j].Numero })
	return partes, nil
}

// Concluir fecha o multipart e confere o tamanho no bucket.
func Concluir(ctx context.Context, arm cloud.Armazenamento, u db.Upload, partes []cloud.Parte) (cloud.Objeto, error) {
	if err := arm.ConcluirEnvio(ctx, u.Key, u.UploadID, partes); err != nil {
		return cloud.Objeto{}, fmt.Errorf("fechando o envio: %w", err)
	}
	obj, err := arm.Info(ctx, u.Key)
	if err != nil {
		return cloud.Objeto{}, err
	}
	if obj.Tamanho != u.Tamanho {
		return cloud.Objeto{}, fmt.Errorf("o bucket tem %d bytes, esperados %d", obj.Tamanho, u.Tamanho)
	}
	return obj, nil
}

// EnviarCaminho sobe um arquivo local para key: PUT simples até 16 MiB,
// multipart acima disso. É o que o worker usa para os derivados.
func EnviarCaminho(ctx context.Context, arm cloud.Armazenamento, key, caminho, contentType string) (cloud.Objeto, error) {
	info, err := os.Stat(caminho)
	if err != nil {
		return cloud.Objeto{}, err
	}
	if info.Size() <= parteMinima {
		dados, err := os.ReadFile(caminho)
		if err != nil {
			return cloud.Objeto{}, err
		}
		if err := arm.Gravar(ctx, key, dados, contentType); err != nil {
			return cloud.Objeto{}, err
		}
		return arm.Info(ctx, key)
	}
	u := db.Upload{Key: key, Tamanho: info.Size(), ParteTamanho: TamanhoDaParte(info.Size())}
	if u.UploadID, err = arm.IniciarEnvio(ctx, key, contentType); err != nil {
		return cloud.Objeto{}, err
	}
	partes, err := EnviarArquivo(ctx, arm, u, caminho, nil)
	if err != nil {
		_ = arm.AbortarEnvio(context.WithoutCancel(ctx), key, u.UploadID)
		return cloud.Objeto{}, err
	}
	return Concluir(ctx, arm, u, partes)
}
