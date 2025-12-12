package aws

import (
	"context"
	"fmt"
	"os"
	"strings"
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

	// Check if this is an SSO profile
	profileManager := NewProfileManager(
		GetDefaultConfigPath(),
		GetDefaultCredentialsPath(),
	)

	if err := profileManager.LoadProfiles(); err != nil {
		logger.Warn("Failed to load profiles for SSO detection", zap.Error(err))
	}

	// Create a custom options slice to handle SSO profiles
	var options []func(*config.LoadOptions) error

	// Add profile and region configuration
	options = append(options,
		config.WithSharedConfigProfile(profile),
		config.WithRegion(region),
	)

	// Enable SSO support by setting the appropriate environment variables if they exist
	if ssoSessionName := os.Getenv("AWS_SSO_SESSION_NAME"); ssoSessionName != "" {
		configFiles := []string{
			os.Getenv("AWS_CONFIG_FILE"),
			os.Getenv("AWS_SHARED_CREDENTIALS_FILE"),
		}
		options = append(options, config.WithSharedConfigFiles(configFiles))
	}

	cfg, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		// Check if this is an SSO profile and provide a specific error message
		if profile, exists := profileManager.GetProfile(profile); exists && profile.IsSSOProfileConfigured() {
			return nil, fmt.Errorf("failed to load AWS config for SSO profile %s. Please run 'aws sso login --profile %s' to authenticate and try again: %w", profile.Name, profile.Name, err)
		}
		// Check if this is an SSO-related error
		if isSSOError(err) {
			return nil, fmt.Errorf("failed to load AWS config for SSO profile %s. Please run 'aws sso login --profile %s' to authenticate and try again: %w", profile, profile, err)
		}
		return nil, fmt.Errorf("failed to load AWS config for profile %s: %w", profile, err)
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
		// Check if this is an SSO-related error
		if isSSOError(err) {
			return nil, fmt.Errorf("failed to get caller identity for SSO profile %s. Please run 'aws sso login --profile %s' to authenticate and try again: %w", profile, profile, err)
		}
		return nil, fmt.Errorf("failed to get caller identity for profile %s: %w", profile, err)
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

	ec2Client := ec2.NewFromConfig(c.config)
	s3Client := s3.NewFromConfig(c.config)
	rdsClient := rds.NewFromConfig(c.config)
	lambdaClient := lambda.NewFromConfig(c.config)
	stsClient := sts.NewFromConfig(c.config)
	cloudWatchLogsClient := cloudwatchlogs.NewFromConfig(c.config)

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

	// Check if this is an SSO profile
	profileManager := NewProfileManager(
		GetDefaultConfigPath(),
		GetDefaultCredentialsPath(),
	)

	if err := profileManager.LoadProfiles(); err != nil {
		logger.Warn("Failed to load profiles for SSO detection", zap.Error(err))
	}

	// Create a custom options slice to handle SSO profiles
	var options []func(*config.LoadOptions) error

	// Add profile and region configuration
	options = append(options,
		config.WithSharedConfigProfile(profile),
		config.WithRegion(region),
	)

	// Enable SSO support by setting the appropriate environment variables if they exist
	if ssoSessionName := os.Getenv("AWS_SSO_SESSION_NAME"); ssoSessionName != "" {
		configFiles := []string{
			os.Getenv("AWS_CONFIG_FILE"),
			os.Getenv("AWS_SHARED_CREDENTIALS_FILE"),
		}
		options = append(options, config.WithSharedConfigFiles(configFiles))
	}

	cfg, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		// Check if this is an SSO profile and provide a specific error message
		if profile, exists := profileManager.GetProfile(profile); exists && profile.IsSSOProfileConfigured() {
			return fmt.Errorf("failed to load AWS config for SSO profile %s. Please run 'aws sso login --profile %s' to authenticate and try again: %w", profile.Name, profile.Name, err)
		}
		// Check if this is an SSO-related error
		if isSSOError(err) {
			return fmt.Errorf("failed to load AWS config for SSO profile %s. Please run 'aws sso login --profile %s' to authenticate and try again: %w", profile, profile, err)
		}
		return fmt.Errorf("failed to load AWS config for profile %s: %w", profile, err)
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
		// Check if this is an SSO-related error
		if isSSOError(err) {
			return fmt.Errorf("failed to get caller identity for SSO profile %s. Please run 'aws sso login --profile %s' to authenticate and try again: %w", profile, profile, err)
		}
		return fmt.Errorf("failed to get caller identity for profile %s: %w", profile, err)
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
		// Check if this is an SSO-related error
		if isSSOError(err) {
			return fmt.Errorf("AWS connection test failed for SSO profile %s. Please run 'aws sso login --profile %s' to authenticate and try again: %w", c.GetProfile(), c.GetProfile(), err)
		}
		return fmt.Errorf("AWS connection test failed for profile %s: %w", c.GetProfile(), err)
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

// isSSOError checks if the error is related to AWS SSO authentication
func isSSOError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	ssoIndicators := []string{
		"SSO",
		"sso",
		"token",
		"expired",
		"login",
		"authenticate",
		"not authorized",
		"access denied",
		"credentials",
	}

	for _, indicator := range ssoIndicators {
		if strings.Contains(errStr, indicator) {
			return true
		}
	}

	return false
}
