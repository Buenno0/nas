// Package aws é o adapter S3 do modo híbrido. É o único lugar do Ozymandias
// que importa o SDK da AWS.
package aws

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/cloudfront/sign"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
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

	sqs *sqs.Client
	sns *sns.Client

	// cdn assina leituras pelo CloudFront. Nil = URL pré-assinada do S3.
	cdn        *sign.URLSigner
	cdnDominio string
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
	b := &bucket{s3: c, assina: s3.NewPresignClient(c), nome: cfg.Bucket, prefixo: cfg.Prefixo,
		sqs: sqs.NewFromConfig(awsCfg), sns: sns.NewFromConfig(awsCfg)}

	if cfg.CDN() {
		// A chave de assinatura só existe em memória: vem do SSM a cada
		// entrada no híbrido e morre com este cliente no kill switch.
		out, err := ssm.NewFromConfig(awsCfg).GetParameter(ctx, &ssm.GetParameterInput{
			Name: aws.String(cfg.CDNParametro), WithDecryption: aws.Bool(true),
		})
		if err != nil {
			return nil, fmt.Errorf("lendo a chave do CloudFront no SSM: %w", err)
		}
		chave, err := sign.LoadPEMPrivKey(strings.NewReader(aws.ToString(out.Parameter.Value)))
		if err != nil {
			return nil, fmt.Errorf("chave do CloudFront inválida: %w", err)
		}
		b.cdn = sign.NewURLSigner(cfg.CDNChaveID, chave)
		b.cdnDominio = strings.TrimSuffix(strings.TrimPrefix(cfg.CDNDominio, "https://"), "/")
	}
	return b, nil
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

func naoExiste(err error) bool {
	var api smithy.APIError
	return errors.As(err, &api) && (api.ErrorCode() == "NotFound" || api.ErrorCode() == "NoSuchKey")
}

func (b *bucket) Info(ctx context.Context, key string) (cloud.Objeto, error) {
	out, err := b.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &b.nome, Key: aws.String(b.chave(key))})
	if err != nil {
		if naoExiste(err) {
			return cloud.Objeto{}, cloud.ErrNaoExiste
		}
		return cloud.Objeto{}, err
	}
	o := cloud.Objeto{Key: key, Tamanho: aws.ToInt64(out.ContentLength), ETag: aws.ToString(out.ETag)}
	// Multipart: o HEAD da parte 1 diz o tamanho de parte usado no envio,
	// que é o que falta para recalcular o ETag de uma cópia local.
	if strings.Contains(o.ETag, "-") {
		p, err := b.s3.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: &b.nome, Key: aws.String(b.chave(key)), PartNumber: aws.Int32(1),
		})
		if err != nil {
			return cloud.Objeto{}, err
		}
		o.TamanhoParte = aws.ToInt64(p.ContentLength)
	}
	return o, nil
}

func (b *bucket) Listar(ctx context.Context, prefixo string, fn func(cloud.Objeto) error) error {
	pag := s3.NewListObjectsV2Paginator(b.s3, &s3.ListObjectsV2Input{
		Bucket: &b.nome, Prefix: aws.String(b.chave(prefixo)),
	})
	for pag.HasMorePages() {
		out, err := pag.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, o := range out.Contents {
			key := aws.ToString(o.Key)
			if b.prefixo != "" {
				key = strings.TrimPrefix(key, b.prefixo+"/")
			}
			if err := fn(cloud.Objeto{Key: key, Tamanho: aws.ToInt64(o.Size), ETag: aws.ToString(o.ETag)}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *bucket) Baixar(ctx context.Context, key string, desde int64) (io.ReadCloser, error) {
	in := &s3.GetObjectInput{Bucket: &b.nome, Key: aws.String(b.chave(key))}
	if desde > 0 {
		in.Range = aws.String(fmt.Sprintf("bytes=%d-", desde))
	}
	out, err := b.s3.GetObject(ctx, in)
	if err != nil {
		if naoExiste(err) {
			return nil, cloud.ErrNaoExiste
		}
		return nil, err
	}
	return out.Body, nil
}

func (b *bucket) Gravar(ctx context.Context, key string, corpo []byte, contentType string) error {
	_, err := b.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &b.nome, Key: aws.String(b.chave(key)), Body: bytes.NewReader(corpo),
		ContentLength: aws.Int64(int64(len(corpo))), ContentType: aws.String(contentType),
	})
	return err
}

func (b *bucket) Apagar(ctx context.Context, key string) error {
	_, err := b.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &b.nome, Key: aws.String(b.chave(key))})
	return err
}

func (b *bucket) Tamanho(ctx context.Context, key string) (int64, error) {
	out, err := b.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &b.nome, Key: aws.String(b.chave(key))})
	if err != nil {
		if naoExiste(err) {
			return 0, cloud.ErrNaoExiste
		}
		return 0, err
	}
	return aws.ToInt64(out.ContentLength), nil
}

func (b *bucket) URLDeLeitura(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if b.cdn != nil {
		u := url.URL{Scheme: "https", Host: b.cdnDominio, Path: "/" + b.chave(key)}
		return b.cdn.Sign(u.String(), time.Now().Add(ttl))
	}
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
