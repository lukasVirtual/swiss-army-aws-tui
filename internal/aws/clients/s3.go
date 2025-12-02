package clients

import (
	"context"
	"fmt"
	"swiss-army-tui/pkg/logger"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

type S3Details struct {
}

type S3Service struct {
	client *s3.Client
}

func (s *S3Service) GetS3Detail(ctx context.Context) ([]S3Details, error) {
	var functions []S3Details

	if s == nil || s.client == nil {
		return nil, fmt.Errorf("s3 service not initialized")
	}

	listOutput, err := s.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	fmt.Println(listOutput)
	if err != nil {
		logger.Error("failed to list s3 functions", zap.Error(err))
		return nil, fmt.Errorf("failed to list s3 functions: %w", err)
	}

	// for _, buckets := range listOutput.Buckets {}

	return functions, nil
}

func NewS3Service(S3Client *s3.Client) (*S3Service, error) {
	if S3Client == nil {
		return nil, fmt.Errorf("No S3 client provided")
	}

	return &S3Service{
		client: S3Client,
	}, nil
}
