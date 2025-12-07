package aws

import (
	"context"
	"fmt"
	"sync"
	"time"

	"swiss-army-tui/internal/aws/clients"
	"swiss-army-tui/pkg/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"go.uber.org/zap"
)

type ServiceClients struct {
	EC2            *clients.EC2Service
	S3             *clients.S3Service
	RDS            *clients.RDSService
	Lambda         *clients.LambdaService
	CloudWatchLogs *clients.CloudWatchLogsService
	STS            *sts.Client
}

type Client struct {
	mu           sync.RWMutex
	config       aws.Config
	clients      *ServiceClients
	profile      string
	region       string
	accountID    string
	userIdentity *sts.GetCallerIdentityOutput
}

func NewClient(profile, region string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Debug("Creating AWS client",
		zap.String("profile", profile),
		zap.String("region", region))

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for profile %s. If you are using an AWS SSO profile, run 'aws sso login --profile %s' and try again: %w", profile, profile, err)
	}

	client := &Client{
		config:  cfg,
		profile: profile,
		region:  region,
	}

	if err := client.initializeClients(); err != nil {
		return nil, fmt.Errorf("failed to initialize AWS service clients: %w", err)
	}

	if err := client.loadCallerIdentity(ctx); err != nil {
		return nil, fmt.Errorf("failed to get caller identity for profile %s. If you are using an AWS SSO profile, run 'aws sso login --profile %s' and try again: %w", profile, profile, err)
	}

	logger.Info("AWS client created successfully",
		zap.String("profile", profile),
		zap.String("region", region),
		zap.String("account_id", client.accountID))

	return client, nil
}

func (c *Client) initializeClients() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Initialize raw AWS SDK clients
	ec2Client := ec2.NewFromConfig(c.config)
	s3Client := s3.NewFromConfig(c.config)
	rdsClient := rds.NewFromConfig(c.config)
	lambdaClient := lambda.NewFromConfig(c.config)
	stsClient := sts.NewFromConfig(c.config)
	cloudWatchLogsClient := cloudwatchlogs.NewFromConfig(c.config)

	// Initialize service wrappers
	ec2Svc, _ := clients.NewEC2Service(ec2Client)
	s3Svc, _ := clients.NewS3Service(s3Client)
	rdsSvc, _ := clients.NewRDSService(rdsClient)
	lambdaSvc, _ := clients.NewLambdaService(lambdaClient)
	cloudWatchLogsSvc, _ := clients.NewCloudWatchLogsService(cloudWatchLogsClient)

	c.clients = &ServiceClients{
		EC2:            ec2Svc,
		S3:             s3Svc,
		RDS:            rdsSvc,
		Lambda:         lambdaSvc,
		CloudWatchLogs: cloudWatchLogsSvc,
		STS:            stsClient,
	}

	return nil
}

func (c *Client) loadCallerIdentity(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	result, err := c.clients.STS.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %w", err)
	}

	c.userIdentity = result
	if result.Account != nil {
		c.accountID = *result.Account
	}

	return nil
}

func (c *Client) GetProfile() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.profile
}

func (c *Client) GetRegion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.region
}

func (c *Client) GetAccountID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.accountID
}

func (c *Client) GetUserIdentity() *sts.GetCallerIdentityOutput {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.userIdentity
}

func (c *Client) GetClients() *ServiceClients {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clients
}

func (c *Client) GetLambdaFunctionDetails(ctx context.Context) ([]clients.LambdaFunctionDetail, error) {
	c.mu.RLock()
	svc := c.clients.Lambda
	c.mu.RUnlock()

	if svc == nil {
		return nil, fmt.Errorf("lambda service not initialized")
	}

	return svc.GetLambdaDetail(ctx)
}

func (c *Client) GetS3FunctionDetails(ctx context.Context) ([]clients.S3Details, error) {
	c.mu.RLock()
	svc := c.clients.S3
	c.mu.RUnlock()

	if svc == nil {
		return nil, fmt.Errorf("S3 service not initialized")
	}

	return svc.GetS3Detail(ctx)
}

func (c *Client) GetEC2FunctionDetails(ctx context.Context) ([]types.Instance, error) {
	c.mu.RLock()
	svc := c.clients.EC2
	c.mu.RUnlock()

	if svc == nil {
		return nil, fmt.Errorf("EC2 service not initialized")
	}

	return svc.GetEC2Detail(ctx)
}

// GetRDSFunctionDetails retrieves details of all RDS instances
func (c *Client) GetRDSFunctionDetails(ctx context.Context) ([]clients.RDSDetails, error) {
	c.mu.RLock()
	svc := c.clients.RDS
	c.mu.RUnlock()

	if svc == nil {
		return nil, fmt.Errorf("RDS service not initialized")
	}

	return svc.GetRDSDetail(ctx)
}

// GetCloudWatchLogsService retrieves the CloudWatch Logs service
func (c *Client) GetCloudWatchLogsService() *clients.CloudWatchLogsService {
	c.mu.RLock()
	svc := c.clients.CloudWatchLogs
	c.mu.RUnlock()
	return svc
}

func (c *Client) SwitchProfile(profile, region string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Debug("Switching AWS profile",
		zap.String("from_profile", c.profile),
		zap.String("to_profile", profile),
		zap.String("region", region))

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
		config.WithRegion(region),
	)
	if err != nil {
		return fmt.Errorf("failed to load AWS config for profile %s. If you are using an AWS SSO profile, run 'aws sso login --profile %s' and try again: %w", profile, profile, err)
	}

	c.mu.Lock()
	c.config = cfg
	c.profile = profile
	c.region = region
	c.mu.Unlock()

	if err := c.initializeClients(); err != nil {
		return fmt.Errorf("failed to reinitialize AWS service clients: %w", err)
	}

	if err := c.loadCallerIdentity(ctx); err != nil {
		return fmt.Errorf("failed to get caller identity for profile %s. If you are using an AWS SSO profile, run 'aws sso login --profile %s' and try again: %w", profile, profile, err)
	}

	logger.Info("AWS profile switched successfully",
		zap.String("profile", profile),
		zap.String("region", region),
		zap.String("account_id", c.accountID))

	return nil
}

func (c *Client) TestConnection(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.clients == nil || c.clients.STS == nil {
		return fmt.Errorf("STS client not initialized")
	}

	_, err := c.clients.STS.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("AWS connection test failed for profile %s. If you are using an AWS SSO profile, run 'aws sso login --profile %s' and try again: %w", c.GetProfile(), c.GetProfile(), err)
	}

	return nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.clients = nil
	c.userIdentity = nil

	logger.Debug("AWS client closed", zap.String("profile", c.profile))
	return nil
}
