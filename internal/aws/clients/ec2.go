package clients

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type EC2Service struct {
	client *ec2.Client
}

func NewEC2Service(EC2Client *ec2.Client) (*EC2Service, error) {
	if EC2Client == nil {
		return nil, fmt.Errorf("EC2 client not provided")
	}

	return &EC2Service{
		client: EC2Client,
	}, nil
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

func (c *EC2Service) StartInstance(ctx context.Context, instanceID string) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("EC2 service not initialized")
	}

	_, err := c.client.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []string{instanceID},
		// TODO: Remove this once we have a way to check if the user has enough permissions
		DryRun: aws.Bool(false),
	})
	if err != nil {
		return fmt.Errorf("failed to start instance: %w", err)
	}
	return nil
}

func (c *EC2Service) StopInstance(ctx context.Context, instanceID string) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("EC2 service not initialized")
	}

	_, err := c.client.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []string{instanceID},
		// TODO: Remove this once we have a way to check if the user has enough permissions
		DryRun: aws.Bool(false),
	})
	if err != nil {
		return fmt.Errorf("failed to stop instance: %w", err)
	}
	return nil
}

func (c *EC2Service) RebootInstance(ctx context.Context, instanceID string) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("EC2 service not initialized")
	}

	_, err := c.client.RebootInstances(ctx, &ec2.RebootInstancesInput{
		InstanceIds: []string{instanceID},
	})
	return err
}

func (c *EC2Service) TerminateInstance(ctx context.Context, instanceID string) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("EC2 service not initialized")
	}

	_, err := c.client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return fmt.Errorf("failed to terminate instance: %w", err)
	}
	return nil
}
