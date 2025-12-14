package clients

import (
	"context"
	"fmt"
	"swiss-army-tui/pkg/logger"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

type S3Details struct {
	Name         string
	CreationDate *time.Time
	Region       string
}

type S3Service struct {
	client *s3.Client
}

func (s *S3Service) GetS3Detail(ctx context.Context) ([]S3Details, error) {
	var details []S3Details

	if s == nil || s.client == nil {
		return nil, fmt.Errorf("s3 service not initialized")
	}

	listOutput, err := s.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		logger.Error("failed to list s3 buckets", zap.Error(err))
		return nil, fmt.Errorf("failed to list s3 buckets: %w", err)
	}

	for _, bucket := range listOutput.Buckets {
		region := ""
		locationOutput, err := s.client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{
			Bucket: bucket.Name,
		})
		name := ""
		if bucket.Name != nil {
			name = *bucket.Name
		}
		if err != nil {
			logger.Debug("failed to get bucket location", zap.String("bucket", name), zap.Error(err))
		} else if locationOutput.LocationConstraint != "" {
			region = string(locationOutput.LocationConstraint)
		}

		detail := S3Details{
			Name:         name,
			CreationDate: bucket.CreationDate,
			Region:       region,
		}
		details = append(details, detail)
	}

	return details, nil
}

func NewS3Service(S3Client *s3.Client) (*S3Service, error) {
	if S3Client == nil {
		return nil, fmt.Errorf("S3 client not provided")
	}

	return &S3Service{
		client: S3Client,
	}, nil
}
