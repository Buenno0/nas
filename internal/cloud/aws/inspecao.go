package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"nas/internal/cloud"
)

// tetoPorPrefixo limita a varredura: um bucket pessoal cabe folgado, e um
// enorme ainda responde em segundos (marcado como truncado).
const tetoPorPrefixo = 50_000

func (b *bucket) Inspecionar(ctx context.Context) cloud.Inspecao {
	in := cloud.Inspecao{Bucket: b.nome, Regiao: b.regiao, Erros: map[string]string{},
		Lifecycle: []cloud.RegraDoBucket{}, CORS: []string{}, Prefixos: []cloud.UsoDoPrefixo{},
		Pendentes: []cloud.EnvioPendente{}}
	falha := func(parte string, err error) { in.Erros[parte] = err.Error() }

	if b.cred != nil {
		if c, err := b.cred.Retrieve(ctx); err != nil {
			falha("credencial", err)
		} else if c.CanExpire {
			in.CredencialExpira = &c.Expires
		}
	}

	if v, err := b.s3.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: &b.nome}); err != nil {
		falha("versionamento", err)
	} else if in.Versionamento = string(v.Status); in.Versionamento == "" {
		in.Versionamento = "Desligado"
	}

	if l, err := b.s3.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: &b.nome}); err != nil {
		falha("lifecycle", err)
	} else {
		for _, r := range l.Rules {
			in.Lifecycle = append(in.Lifecycle, cloud.RegraDoBucket{
				ID: aws.ToString(r.ID), Ativa: r.Status == "Enabled", Resumo: resumoDaRegra(r),
				Prefixo: prefixoDaRegra(r),
			})
		}
	}

	if c, err := b.s3.GetBucketCors(ctx, &s3.GetBucketCorsInput{Bucket: &b.nome}); err != nil {
		falha("cors", err)
	} else {
		for _, r := range c.CORSRules {
			in.CORS = append(in.CORS, r.AllowedOrigins...)
		}
	}

	if err := b.usoPorPrefixo(ctx, &in); err != nil {
		falha("objetos", err)
	}

	pag := s3.NewListMultipartUploadsPaginator(b.s3, &s3.ListMultipartUploadsInput{Bucket: &b.nome, Prefix: aws.String(b.prefixo)})
	for pag.HasMorePages() && len(in.Pendentes) < 200 {
		out, err := pag.NextPage(ctx)
		if err != nil {
			falha("multiparts", err)
			break
		}
		for _, u := range out.Uploads {
			in.Pendentes = append(in.Pendentes, cloud.EnvioPendente{Key: b.semPrefixo(aws.ToString(u.Key)), Iniciado: aws.ToTime(u.Initiated)})
		}
	}
	return in
}

func (b *bucket) semPrefixo(key string) string {
	if b.prefixo == "" {
		return key
	}
	return strings.TrimPrefix(key, b.prefixo+"/")
}

// usoPorPrefixo descobre os prefixos de primeiro nível (midia/, derivados/,
// catalogo/…) e soma cada um, com a contagem por classe de armazenamento.
func (b *bucket) usoPorPrefixo(ctx context.Context, in *cloud.Inspecao) error {
	raiz := ""
	if b.prefixo != "" {
		raiz = b.prefixo + "/"
	}
	topo, err := b.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: &b.nome, Prefix: &raiz, Delimiter: aws.String("/")})
	if err != nil {
		return err
	}
	prefixos := []string{}
	for _, p := range topo.CommonPrefixes {
		prefixos = append(prefixos, aws.ToString(p.Prefix))
	}
	if len(topo.Contents) > 0 {
		prefixos = append(prefixos, "") // objetos soltos na raiz
	}
	for _, p := range prefixos {
		uso := cloud.UsoDoPrefixo{Prefixo: b.semPrefixo(p), Classes: map[string]int64{}}
		if p == "" {
			uso.Prefixo = "(raiz)"
			for _, o := range topo.Contents {
				uso.Bytes += aws.ToInt64(o.Size)
				uso.Objetos++
				uso.Classes[string(o.StorageClass)]++
			}
			in.Prefixos = append(in.Prefixos, uso)
			continue
		}
		pag := s3.NewListObjectsV2Paginator(b.s3, &s3.ListObjectsV2Input{Bucket: &b.nome, Prefix: aws.String(p)})
		for pag.HasMorePages() {
			if uso.Objetos >= tetoPorPrefixo {
				uso.Truncado = true
				break
			}
			out, err := pag.NextPage(ctx)
			if err != nil {
				return err
			}
			for _, o := range out.Contents {
				uso.Bytes += aws.ToInt64(o.Size)
				uso.Objetos++
				uso.Classes[string(o.StorageClass)]++
			}
		}
		in.Prefixos = append(in.Prefixos, uso)
	}
	sort.Slice(in.Prefixos, func(i, j int) bool { return in.Prefixos[i].Bytes > in.Prefixos[j].Bytes })
	return nil
}

func prefixoDaRegra(r s3types.LifecycleRule) string {
	if r.Filter != nil {
		if r.Filter.Prefix != nil {
			return aws.ToString(r.Filter.Prefix)
		}
		if r.Filter.And != nil {
			return aws.ToString(r.Filter.And.Prefix)
		}
	}
	return aws.ToString(r.Prefix) //nolint:staticcheck // regras antigas
}

// resumoDaRegra traduz a regra para uma linha legível.
func resumoDaRegra(r s3types.LifecycleRule) string {
	var partes []string
	if a := r.AbortIncompleteMultipartUpload; a != nil {
		partes = append(partes, fmt.Sprintf("aborta multipart após %d dias", aws.ToInt32(a.DaysAfterInitiation)))
	}
	for _, t := range r.Transitions {
		if aws.ToInt32(t.Days) == 0 {
			partes = append(partes, fmt.Sprintf("vai direto para %s", t.StorageClass))
		} else {
			partes = append(partes, fmt.Sprintf("vai para %s após %d dias", t.StorageClass, aws.ToInt32(t.Days)))
		}
	}
	if e := r.Expiration; e != nil && aws.ToInt32(e.Days) > 0 {
		partes = append(partes, fmt.Sprintf("expira após %d dias", aws.ToInt32(e.Days)))
	}
	if e := r.NoncurrentVersionExpiration; e != nil {
		partes = append(partes, fmt.Sprintf("versões antigas somem após %d dias", aws.ToInt32(e.NoncurrentDays)))
	}
	if len(partes) == 0 {
		return "sem ações reconhecidas"
	}
	return strings.Join(partes, "; ")
}

func (b *bucket) EstadoDaFila(ctx context.Context, url string) (cloud.EstadoDaFila, error) {
	out, err := b.sqs.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: &url,
		AttributeNames: []sqstypes.QueueAttributeName{
			sqstypes.QueueAttributeNameApproximateNumberOfMessages,
			sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
			sqstypes.QueueAttributeNameApproximateNumberOfMessagesDelayed,
			sqstypes.QueueAttributeNameRedrivePolicy,
		},
	})
	if err != nil {
		return cloud.EstadoDaFila{}, err
	}
	n := func(k sqstypes.QueueAttributeName) int64 {
		v, _ := strconv.ParseInt(out.Attributes[string(k)], 10, 64)
		return v
	}
	e := cloud.EstadoDaFila{
		Nome:      path.Base(url),
		Visiveis:  n(sqstypes.QueueAttributeNameApproximateNumberOfMessages),
		EmVoo:     n(sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible),
		Atrasadas: n(sqstypes.QueueAttributeNameApproximateNumberOfMessagesDelayed),
	}
	// A DLQ vem como ARN na redrive policy; a URL sai dele.
	var redrive struct {
		Alvo string `json:"deadLetterTargetArn"`
	}
	if json.Unmarshal([]byte(out.Attributes[string(sqstypes.QueueAttributeNameRedrivePolicy)]), &redrive) == nil && redrive.Alvo != "" {
		// arn:aws:sqs:regiao:conta:nome → https://sqs.regiao.amazonaws.com/conta/nome
		if p := strings.Split(redrive.Alvo, ":"); len(p) == 6 {
			e.DLQ = strings.TrimSuffix(url, path.Base(url)) + p[5]
		}
	}
	return e, nil
}
