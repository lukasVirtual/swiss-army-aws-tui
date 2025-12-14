package clients

import (
	"context"
	"fmt"
	"time"

	"swiss-army-tui/pkg/logger"

	"github.com/aws/aws-sdk-go-v2/service/rds"
	"go.uber.org/zap"
)

// RDSDetails represents the details of an RDS instance
type RDSDetails struct {
	DBInstanceIdentifier string
	Engine               string
	EngineVersion        string
	DBInstanceStatus     string
	Endpoint             string
	AllocatedStorage     int32
	InstanceCreateTime   *time.Time
	Region               string
}

// RDSService wraps the RDS client and provides high-level operations
type RDSService struct {
	client *rds.Client
}

// GetRDSDetail retrieves details of all RDS instances
func (s *RDSService) GetRDSDetail(ctx context.Context) ([]RDSDetails, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("RDS service not initialized")
	}

	var allInstances []RDSDetails

	// Create input for DescribeDBInstances
	input := &rds.DescribeDBInstancesInput{}

	// Use paginator to handle cases with many RDS instances
	paginator := rds.NewDescribeDBInstancesPaginator(s.client, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			logger.Error("failed to describe RDS instances", zap.Error(err))
			return nil, fmt.Errorf("failed to describe RDS instances: %w", err)
		}

		for _, dbInstance := range output.DBInstances {
			detail := RDSDetails{
				DBInstanceIdentifier: getStringValue(dbInstance.DBInstanceIdentifier),
				Engine:               getStringValue(dbInstance.Engine),
				EngineVersion:        getStringValue(dbInstance.EngineVersion),
				DBInstanceStatus:     getStringValue(dbInstance.DBInstanceStatus),
				AllocatedStorage:     getInt32Value(dbInstance.AllocatedStorage),
				InstanceCreateTime:   dbInstance.InstanceCreateTime,
			}

			// Get the endpoint
			if dbInstance.Endpoint != nil {
				detail.Endpoint = fmt.Sprintf("%s:%d",
					getStringValue(dbInstance.Endpoint.Address),
					getInt32Value(dbInstance.Endpoint.Port),
				)
			}

			allInstances = append(allInstances, detail)
		}
	}

	return allInstances, nil
}

// NewRDSService creates a new RDSService instance
func NewRDSService(client *rds.Client) (*RDSService, error) {
	if client == nil {
		return nil, fmt.Errorf("RDS client not provided")
	}

	return &RDSService{
		client: client,
	}, nil
}

// Helper function to safely get string value from a string pointer
func getStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Helper function to safely get int32 value from an int32 pointer
func getInt32Value(i *int32) int32 {
	if i == nil {
		return 0
	}
	return *i
}

// formatTime formats a time.Time pointer as a string
func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}
