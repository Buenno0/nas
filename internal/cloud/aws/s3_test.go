package aws

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"nas/internal/cloud"
	"nas/internal/config"
)

// Teste de integração contra um S3 de verdade, sem AWS: MinIO nativo.
//
//	brew install minio
//	minio server /tmp/minio &   # usuário/senha padrão minioadmin
//	mc mb local/ozymandias-teste (ou pelo console em :9001)
//	AWS_ACCESS_KEY_ID=minioadmin AWS_SECRET_ACCESS_KEY=minioadmin \
//	NAS_S3_ENDPOINT=http://127.0.0.1:9000 NAS_S3_BUCKET=ozymandias-teste \
//	  go test ./internal/cloud/aws -run S3 -v
func TestS3MultipartDePontaAPonta(t *testing.T) {
	endpoint, bucket := os.Getenv("NAS_S3_ENDPOINT"), os.Getenv("NAS_S3_BUCKET")
	if endpoint == "" || bucket == "" {
		t.Skip("defina NAS_S3_ENDPOINT e NAS_S3_BUCKET (MinIO) para rodar")
	}
	cfg := config.Nuvem{Regiao: "us-east-1", Bucket: bucket, Endpoint: endpoint, PathStyle: true}
	chave := cloud.Nova(func() config.Nuvem { return cfg }, false)
	ctx := context.Background()
	criarBucketSeFaltar(t, ctx, cfg)
	if err := chave.Ativar(ctx); err != nil {
		t.Fatal(err)
	}
	arm, _, _ := chave.Hibrido()

	key := fmt.Sprintf("bibliotecas/teste/%d/arquivo.bin", time.Now().UnixNano())
	dados := make([]byte, 5<<20+123) // duas partes: 5 MiB (mínimo do S3) + resto
	rand.Read(dados)

	id, err := arm.IniciarEnvio(ctx, key, "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	e1, err := arm.EnviarParte(ctx, key, id, 1, bytes.NewReader(dados[:5<<20]), 5<<20)
	if err != nil {
		t.Fatal(err)
	}

	// A segunda parte vai pela URL assinada, como o navegador faz.
	url, err := arm.URLDaParte(ctx, key, id, 2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(dados[5<<20:]))
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("PUT assinado: %v %v", err, resp)
	}
	e2 := resp.Header.Get("ETag")

	partes, err := arm.PartesEnviadas(ctx, key, id)
	if err != nil || len(partes) != 2 {
		t.Fatalf("partes = %v, %v", partes, err)
	}
	if err := arm.ConcluirEnvio(ctx, key, id, []cloud.Parte{{Numero: 1, ETag: e1}, {Numero: 2, ETag: e2}}); err != nil {
		t.Fatal(err)
	}
	if n, err := arm.Tamanho(ctx, key); err != nil || n != int64(len(dados)) {
		t.Fatalf("tamanho = %d, %v", n, err)
	}

	// O ETag recalculado sobre a cópia local precisa bater com o do bucket:
	// é a prova que "liberar espaço" usa antes de apagar qualquer coisa.
	obj, err := arm.Info(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir() + "/copia.bin"
	os.WriteFile(tmp, dados, 0o644)
	local, err := cloud.ETagLocal(tmp, obj.TamanhoParte)
	if err != nil || !cloud.MesmoConteudo(local, obj.ETag) {
		t.Fatalf("ETag local %q ≠ bucket %q (parte %d): %v", local, obj.ETag, obj.TamanhoParte, err)
	}

	leitura, err := arm.URLDeLeitura(ctx, key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.Get(leitura)
	if err != nil {
		t.Fatal(err)
	}
	lido, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Equal(lido, dados) {
		t.Fatal("o conteúdo lido difere do enviado")
	}

	// Kill switch: depois dele, o adapter antigo não consegue falar.
	chave.Desligar()
	if _, err := arm.Tamanho(ctx, key); err == nil {
		t.Fatal("o adapter falou com o bucket depois do kill switch")
	}
	if chave.Bloqueadas() == 0 {
		t.Fatal("nuvem_bloqueadas_total não contou a tentativa")
	}
}

// criarBucketSeFaltar dispensa o cliente mc: num MinIO recém-subido, o bucket
// de teste ainda não existe. O adapter do Ozymandias nunca cria bucket (isso é
// do OpenTofu), então o teste fala com o SDK direto.
func criarBucketSeFaltar(t *testing.T, ctx context.Context, cfg config.Nuvem) {
	t.Helper()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Regiao))
	if err != nil {
		t.Fatal(err)
	}
	c := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true
	})
	if _, err := c.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &cfg.Bucket}); err == nil {
		return
	}
	if _, err := c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &cfg.Bucket}); err != nil {
		t.Fatalf("criando o bucket de teste: %v", err)
	}
}
