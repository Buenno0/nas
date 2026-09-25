package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

// AcordarWorker põe o serviço em pelo menos 1 réplica. Nunca diminui: se o
// autoscaling já subiu mais, fica como está. Quem desliga depois é o próprio
// autoscaling, quando a fila esvazia.
func (b *bucket) AcordarWorker(ctx context.Context, cluster, servico string) error {
	c := ecs.NewFromConfig(b.awsCfg)
	out, err := c.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: &cluster, Services: []string{servico}})
	if err != nil {
		return err
	}
	if len(out.Services) == 0 || out.Services[0].DesiredCount >= 1 {
		return nil
	}
	_, err = c.UpdateService(ctx, &ecs.UpdateServiceInput{Cluster: &cluster, Service: &servico, DesiredCount: aws.Int32(1)})
	return err
}
