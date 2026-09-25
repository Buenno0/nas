// Package aws é o adapter S3 do modo híbrido. É o único lugar do Ozymandias
// que importa o SDK da AWS.
package aws

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"nas/internal/cloud"
	"nas/internal/config"
)

func init() { cloud.Registrar(Conectar) }

type bucket struct {
	s3      *s3.Client
	assina  *s3.PresignClient
	nome    string
	prefixo string
}

// Conectar monta o cliente. Credenciais vêm da cadeia padrão (ou do perfil
// configurado, tipicamente um credential_process do Roles Anywhere) e ficam só
// em memória: quando o kill switch descarta este cliente, elas vão junto.
func Conectar(ctx context.Context, cfg config.Nuvem, cliente *http.Client) (cloud.Armazenamento, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Regiao),
		awsconfig.WithHTTPClient(cliente),
	}
	if cfg.Perfil != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(cfg.Perfil))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("carregando credenciais: %w", err)
	}
	c := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.PathStyle
	})
	return &bucket{s3: c, assina: s3.NewPresignClient(c), nome: cfg.Bucket, prefixo: cfg.Prefixo}, nil
}

func (b *bucket) chave(key string) string {
	if b.prefixo == "" {
		return key
	}
	return path.Join(b.prefixo, key)
}

func (b *bucket) Verificar(ctx context.Context) error {
	_, err := b.s3.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &b.nome})
	return err
}

func (b *bucket) Tamanho(ctx context.Context, key string) (int64, error) {
	out, err := b.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &b.nome, Key: aws.String(b.chave(key))})
	if err != nil {
		var api smithy.APIError
		if errors.As(err, &api) && (api.ErrorCode() == "NotFound" || api.ErrorCode() == "NoSuchKey") {
			return 0, cloud.ErrNaoExiste
		}
		return 0, err
	}
	return aws.ToInt64(out.ContentLength), nil
}

func (b *bucket) URLDeLeitura(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := b.assina.PresignGetObject(ctx,
		&s3.GetObjectInput{Bucket: &b.nome, Key: aws.String(b.chave(key))},
		s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (b *bucket) IniciarEnvio(ctx context.Context, key, contentType string) (string, error) {
	in := &s3.CreateMultipartUploadInput{Bucket: &b.nome, Key: aws.String(b.chave(key))}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	out, err := b.s3.CreateMultipartUpload(ctx, in)
	if err != nil {
		return "", err
	}
	return aws.ToString(out.UploadId), nil
}

func (b *bucket) URLDaParte(ctx context.Context, key, uploadID string, n int32, ttl time.Duration) (string, error) {
	req, err := b.assina.PresignUploadPart(ctx, &s3.UploadPartInput{
		Bucket: &b.nome, Key: aws.String(b.chave(key)), UploadId: &uploadID, PartNumber: &n,
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (b *bucket) EnviarParte(ctx context.Context, key, uploadID string, n int32, corpo io.ReadSeeker, tamanho int64) (string, error) {
	out, err := b.s3.UploadPart(ctx, &s3.UploadPartInput{
		Bucket: &b.nome, Key: aws.String(b.chave(key)), UploadId: &uploadID,
		PartNumber: &n, Body: corpo, ContentLength: &tamanho,
	})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.ETag), nil
}

func (b *bucket) PartesEnviadas(ctx context.Context, key, uploadID string) ([]cloud.Parte, error) {
	var (
		partes []cloud.Parte
		marca  *string
	)
	for {
		out, err := b.s3.ListParts(ctx, &s3.ListPartsInput{
			Bucket: &b.nome, Key: aws.String(b.chave(key)), UploadId: &uploadID, PartNumberMarker: marca,
		})
		if err != nil {
			return nil, err
		}
		for _, p := range out.Parts {
			partes = append(partes, cloud.Parte{Numero: aws.ToInt32(p.PartNumber), ETag: aws.ToString(p.ETag)})
		}
		if !aws.ToBool(out.IsTruncated) {
			return partes, nil
		}
		marca = out.NextPartNumberMarker
	}
}

func (b *bucket) ConcluirEnvio(ctx context.Context, key, uploadID string, partes []cloud.Parte) error {
	completas := make([]types.CompletedPart, len(partes))
	for i, p := range partes {
		completas[i] = types.CompletedPart{PartNumber: aws.Int32(p.Numero), ETag: aws.String(p.ETag)}
	}
	_, err := b.s3.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket: &b.nome, Key: aws.String(b.chave(key)), UploadId: &uploadID,
		MultipartUpload: &types.CompletedMultipartUpload{Parts: completas},
	})
	return err
}

func (b *bucket) AbortarEnvio(ctx context.Context, key, uploadID string) error {
	_, err := b.s3.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket: &b.nome, Key: aws.String(b.chave(key)), UploadId: &uploadID,
	})
	return err
}
