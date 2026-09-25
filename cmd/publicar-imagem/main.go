// publicar-imagem constrói a imagem da nuvem na própria AWS (CodeBuild) e a
// envia ao ECR. O Mac não precisa de Docker: ele só empacota o código com
// `git archive`, sobe o zip para o bucket de build e acompanha o build.
//
//	go run ./cmd/publicar-imagem --bucket <bucket_build> --projeto <projeto_build>
//
// (o Makefile tira os dois do `tofu output`). As credenciais vêm da cadeia
// padrão da AWS; use AWS_PROFILE com um perfil que possa disparar o build.
// Fica fora do binário `nas` de propósito: é ferramenta de operação, e o SDK
// do CodeBuild não tem o que fazer num servidor de mídia.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	cbtypes "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func main() {
	bucket := flag.String("bucket", "", "bucket de build (tofu output -raw bucket_build)")
	projeto := flag.String("projeto", "", "projeto do CodeBuild (tofu output -raw projeto_build)")
	regiao := flag.String("regiao", "", "região (padrão: a do perfil)")
	flag.Parse()
	if *bucket == "" || *projeto == "" {
		fmt.Fprintln(os.Stderr, "uso: publicar-imagem --bucket <bucket_build> --projeto <projeto_build>")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := publicar(ctx, *bucket, *projeto, *regiao); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func publicar(ctx context.Context, bucket, projeto, regiao string) error {
	// Só o que está commitado vai: um build reproduzível a partir do HEAD, e
	// nada do working tree (segredos locais, arquivos soltos) sai do Mac.
	if sujo, _ := exec.Command("git", "status", "--porcelain").Output(); len(bytes.TrimSpace(sujo)) > 0 {
		fmt.Println("aviso: há mudanças sem commit; o build usa o último commit, sem elas.")
	}
	versao := saida("git", "describe", "--tags", "--always")
	if versao == "" {
		versao = "dev"
	}
	zip, err := exec.CommandContext(ctx, "git", "archive", "--format=zip", "HEAD").Output()
	if err != nil {
		return fmt.Errorf("git archive: %w", err)
	}
	fmt.Printf("fonte: %s (%d KB)\n", versao, len(zip)>>10)

	opts := []func(*awsconfig.LoadOptions) error{}
	if regiao != "" {
		opts = append(opts, awsconfig.WithRegion(regiao))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("fontes/%s-%d.zip", versao, time.Now().Unix())
	if _, err := s3.NewFromConfig(cfg).PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucket, Key: &key, Body: bytes.NewReader(zip), ContentType: aws.String("application/zip"),
	}); err != nil {
		return fmt.Errorf("enviando a fonte: %w", err)
	}

	cb := codebuild.NewFromConfig(cfg)
	inicio, err := cb.StartBuild(ctx, &codebuild.StartBuildInput{
		ProjectName:            &projeto,
		SourceTypeOverride:     cbtypes.SourceTypeS3,
		SourceLocationOverride: aws.String(bucket + "/" + key),
		EnvironmentVariablesOverride: []cbtypes.EnvironmentVariable{
			{Name: aws.String("VERSAO"), Value: aws.String(versao), Type: cbtypes.EnvironmentVariableTypePlaintext},
		},
	})
	if err != nil {
		return fmt.Errorf("disparando o build: %w", err)
	}
	id := aws.ToString(inicio.Build.Id)
	fmt.Printf("build %s no ar\n", id)

	ultima := ""
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nparei de acompanhar; o build continua na AWS.")
			return nil
		case <-time.After(10 * time.Second):
		}
		out, err := cb.BatchGetBuilds(ctx, &codebuild.BatchGetBuildsInput{Ids: []string{id}})
		if err != nil || len(out.Builds) == 0 {
			continue
		}
		b := out.Builds[0]
		fase := aws.ToString(b.CurrentPhase)
		if fase != ultima {
			fmt.Printf("  %s\n", strings.ToLower(fase))
			ultima = fase
		}
		switch b.BuildStatus {
		case cbtypes.StatusTypeSucceeded:
			fmt.Printf("pronto: imagem %s e latest no ECR\n", versao)
			return nil
		case cbtypes.StatusTypeInProgress:
			continue
		default:
			link := ""
			if b.Logs != nil {
				link = aws.ToString(b.Logs.DeepLink)
			}
			return fmt.Errorf("build terminou em %s; logs: %s", b.BuildStatus, link)
		}
	}
}

func saida(nome string, args ...string) string {
	out, err := exec.Command(nome, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
