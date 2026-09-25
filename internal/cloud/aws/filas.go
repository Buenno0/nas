package aws

import (
	"context"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"nas/internal/cloud"
)

func (b *bucket) EnviarMensagem(ctx context.Context, fila string, corpo []byte) error {
	_, err := b.sqs.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: &fila, MessageBody: aws.String(string(corpo))})
	return err
}

func (b *bucket) ReceberMensagens(ctx context.Context, fila string, max int32, espera time.Duration) ([]cloud.Mensagem, error) {
	out, err := b.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            &fila,
		MaxNumberOfMessages: max,
		WaitTimeSeconds:     int32(espera.Seconds()),
		MessageSystemAttributeNames: []sqstypes.MessageSystemAttributeName{
			sqstypes.MessageSystemAttributeNameApproximateReceiveCount,
		},
	})
	if err != nil {
		return nil, err
	}
	msgs := make([]cloud.Mensagem, 0, len(out.Messages))
	for _, m := range out.Messages {
		n, _ := strconv.Atoi(m.Attributes[string(sqstypes.MessageSystemAttributeNameApproximateReceiveCount)])
		msgs = append(msgs, cloud.Mensagem{
			ID: aws.ToString(m.MessageId), Recibo: aws.ToString(m.ReceiptHandle),
			Corpo: []byte(aws.ToString(m.Body)), Recebimentos: n,
		})
	}
	return msgs, nil
}

func (b *bucket) ApagarMensagem(ctx context.Context, fila, recibo string) error {
	_, err := b.sqs.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: &fila, ReceiptHandle: &recibo})
	return err
}

func (b *bucket) EstenderVisibilidade(ctx context.Context, fila, recibo string, d time.Duration) error {
	_, err := b.sqs.ChangeMessageVisibility(ctx, &sqs.ChangeMessageVisibilityInput{
		QueueUrl: &fila, ReceiptHandle: &recibo, VisibilityTimeout: int32(d.Seconds()),
	})
	return err
}

func (b *bucket) Publicar(ctx context.Context, topico string, corpo []byte, origem string) error {
	_, err := b.sns.Publish(ctx, &sns.PublishInput{
		TopicArn: &topico, Message: aws.String(string(corpo)),
		MessageAttributes: map[string]snstypes.MessageAttributeValue{
			"origem": {DataType: aws.String("String"), StringValue: aws.String(origem)},
		},
	})
	return err
}
