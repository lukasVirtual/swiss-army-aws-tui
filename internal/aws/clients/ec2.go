package clients

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type EC2Service struct {
	client *ec2.Client
}

func (c *EC2Service) GetEC2Detail(ctx context.Context) ([]types.Instance, error) {
	var allInstances []types.Instance
	input := &ec2.DescribeInstancesInput{}
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("EC2 service not initialized")
	}

	paginator := ec2.NewDescribeInstancesPaginator(c.client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get page: %w", err)
		}

		for _, reservation := range output.Reservations {
			allInstances = append(allInstances, reservation.Instances...)
		}
	}

	return allInstances, nil
}

func NewEC2Service(EC2Client *ec2.Client) (*EC2Service, error) {
	if EC2Client == nil {
		return nil, fmt.Errorf("EC2 client not provided")
	}

	return &EC2Service{
		client: EC2Client,
	}, nil
}
