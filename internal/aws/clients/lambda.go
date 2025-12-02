package clients

import (
	"context"
	"fmt"
	"log"
	"swiss-army-tui/pkg/logger"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"go.uber.org/zap"
)

type LambdaFunctionDetail struct {
	FunctionName     string
	Runtime          string
	Handler          string
	MemorySize       int32
	Timeout          int32
	SnapStartEnabled bool
	SnapStartStatus  string
	State            string
	LastModified     string
	Description      string
	CodeSize         int64
}

type LambdaService struct {
	client *lambda.Client
}

func NewLambdaService(lambdaClient *lambda.Client) (*LambdaService, error) {
	if lambdaClient == nil {
		return nil, fmt.Errorf("No Lambda client provided")
	}

	return &LambdaService{
		client: lambdaClient,
	}, nil
}

func (c *LambdaService) GetLambdaDetail(ctx context.Context) ([]LambdaFunctionDetail, error) {
	var functions []LambdaFunctionDetail

	if c == nil || c.client == nil {
		return nil, fmt.Errorf("lambda service not initialized")
	}

	listOutput, err := c.client.ListFunctions(ctx, &lambda.ListFunctionsInput{})
	if err != nil {
		logger.Error("failed to list Lambda functions", zap.Error(err))
		return nil, fmt.Errorf("failed to list Lambda functions: %w", err)
	}

	for _, fn := range listOutput.Functions {
		detail, err := c.client.GetFunctionConfiguration(ctx, &lambda.GetFunctionConfigurationInput{
			FunctionName: fn.FunctionName,
		})
		if err != nil {
			if fn.FunctionName != nil {
				log.Printf("Error getting details for %s: %v", *fn.FunctionName, err)
			} else {
				log.Printf("Error getting function details: %v", err)
			}
			continue
		}

		// Extract SnapStart information
		snapStartEnabled := false
		snapStartStatus := "Not Available"

		if detail.SnapStart != nil {
			snapStartEnabled = detail.SnapStart.ApplyOn == types.SnapStartApplyOnPublishedVersions
			if detail.SnapStart.OptimizationStatus != "" {
				snapStartStatus = string(detail.SnapStart.OptimizationStatus)
			}
		}

		functions = append(functions, LambdaFunctionDetail{
			FunctionName:     safeString(detail.FunctionName),
			Runtime:          string(detail.Runtime),
			Handler:          safeString(detail.Handler),
			MemorySize:       safeInt32(detail.MemorySize),
			Timeout:          safeInt32(detail.Timeout),
			SnapStartEnabled: snapStartEnabled,
			SnapStartStatus:  snapStartStatus,
			State:            string(detail.State),
			LastModified:     safeString(detail.LastModified),
			Description:      safeString(detail.Description),
			CodeSize:         detail.CodeSize,
		})
	}

	return functions, nil
}

func safeString(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func safeInt32(ptr *int32) int32 {
	if ptr == nil {
		return 0
	}
	return *ptr
}
