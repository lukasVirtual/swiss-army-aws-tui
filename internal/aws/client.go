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
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"go.uber.org/zap"
)

// ServiceClients holds all AWS service clients
type ServiceClients struct {
	EC2    *ec2.Client
	S3     *s3.Client
	RDS    *rds.Client
	ECS    *ecs.Client
	Lambda *lambda.Client
	STS    *sts.Client
}

// Client represents an AWS client manager
type Client struct {
	mu            sync.RWMutex
	config        aws.Config
	clients       *ServiceClients
	lambdaService *clients.LambdaService
	s3Service     *clients.S3Service
	profile       string
	region        string
	accountID     string
	userIdentity  *sts.GetCallerIdentityOutput
}

// NewClient creates a new AWS client with the specified profile
func NewClient(profile, region string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Debug("Creating AWS client",
		zap.String("profile", profile),
		zap.String("region", region))

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for profile %s: %w", profile, err)
	}

	client := &Client{
		config:  cfg,
		profile: profile,
		region:  region,
	}

	// Initialize service clients
	if err := client.initializeClients(); err != nil {
		return nil, fmt.Errorf("failed to initialize AWS service clients: %w", err)
	}

	// Get caller identity
	if err := client.loadCallerIdentity(ctx); err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %w", err)
	}

	logger.Info("AWS client created successfully",
		zap.String("profile", profile),
		zap.String("region", region),
		zap.String("account_id", client.accountID))

	return client, nil
}

// initializeClients initializes all AWS service clients
func (c *Client) initializeClients() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.clients = &ServiceClients{
		EC2:    ec2.NewFromConfig(c.config),
		S3:     s3.NewFromConfig(c.config),
		RDS:    rds.NewFromConfig(c.config),
		ECS:    ecs.NewFromConfig(c.config),
		Lambda: lambda.NewFromConfig(c.config),
		STS:    sts.NewFromConfig(c.config),
	}

	// Initialize higher-level service wrappers that depend on the raw SDK clients.
	// Create Lambda service wrapper by passing the already-created raw Lambda SDK client
	// (c.clients.Lambda). This avoids an import cycle because the clients package only
	// depends on the raw AWS SDK, not on this aws package.
	if c.clients != nil && c.clients.Lambda != nil {
		lambdaSvc, err := clients.NewLambdaService(c.clients.Lambda)
		if err != nil {
			logger.Debug("failed to initialize lambda service wrapper", zap.Error(err))
		} else {
			c.lambdaService = lambdaSvc
		}
	} else {
		logger.Debug("lambda SDK client not initialized; skipping lambda service wrapper")
	}

	if c.clients != nil && c.clients.S3 != nil {
		S3Svc, err := clients.NewS3Service(c.clients.S3)
		if err != nil {
			logger.Debug("failed to initialize S3 service wrapper", zap.Error(err))
		} else {
			c.s3Service = S3Svc
		}
	} else {
		logger.Debug("S3 SDK client not initialized; skipping S3 service wrapper")
	}
	return nil
}

// loadCallerIdentity loads the caller identity and account information
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

// GetProfile returns the current AWS profile
func (c *Client) GetProfile() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.profile
}

// GetRegion returns the current AWS region
func (c *Client) GetRegion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.region
}

// GetAccountID returns the current AWS account ID
func (c *Client) GetAccountID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.accountID
}

// GetUserIdentity returns the caller identity information
func (c *Client) GetUserIdentity() *sts.GetCallerIdentityOutput {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.userIdentity
}

// GetClients returns the service clients
func (c *Client) GetClients() *ServiceClients {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clients
}

// GetLambdaFunctionDetails returns detailed lambda function metadata via the Lambda service wrapper.
// It returns an error if the Lambda service wrapper is not initialized.
func (c *Client) GetLambdaFunctionDetails(ctx context.Context) ([]clients.LambdaFunctionDetail, error) {
	c.mu.RLock()
	svc := c.lambdaService
	c.mu.RUnlock()

	if svc == nil {
		return nil, fmt.Errorf("lambda service not initialized")
	}

	return svc.GetLambdaDetail(ctx)
}

// GetLambdaService returns the higher-level Lambda service wrapper.
// May be nil if initialization failed.
func (c *Client) GetLambdaService() *clients.LambdaService {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lambdaService
}

// GetLambdaFunctionDetails returns detailed lambda function metadata via the Lambda service wrapper.
// It returns an error if the Lambda service wrapper is not initialized.
func (c *Client) GetS3FunctionDetails(ctx context.Context) ([]clients.S3Details, error) {
	c.mu.RLock()
	svc := c.s3Service
	c.mu.RUnlock()

	if svc == nil {
		return nil, fmt.Errorf("S3 service not initialized")
	}

	return svc.GetS3Detail(ctx)
}

// GetS3Service returns the higher-level S3 service wrapper.
// May be nil if initialization failed.
func (c *Client) GetS3Service() *clients.S3Service {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.s3Service
}

// SwitchProfile switches to a different AWS profile
func (c *Client) SwitchProfile(profile, region string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Debug("Switching AWS profile",
		zap.String("from_profile", c.profile),
		zap.String("to_profile", profile),
		zap.String("region", region))

	// Load new config
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
		config.WithRegion(region),
	)
	if err != nil {
		return fmt.Errorf("failed to load AWS config for profile %s: %w", profile, err)
	}

	c.mu.Lock()
	c.config = cfg
	c.profile = profile
	c.region = region
	c.mu.Unlock()

	// Reinitialize clients
	if err := c.initializeClients(); err != nil {
		return fmt.Errorf("failed to reinitialize AWS service clients: %w", err)
	}

	// Reload caller identity
	if err := c.loadCallerIdentity(ctx); err != nil {
		return fmt.Errorf("failed to get caller identity for new profile: %w", err)
	}

	logger.Info("AWS profile switched successfully",
		zap.String("profile", profile),
		zap.String("region", region),
		zap.String("account_id", c.accountID))

	return nil
}

// TestConnection tests the AWS connection by calling STS GetCallerIdentity
func (c *Client) TestConnection(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.clients == nil || c.clients.STS == nil {
		return fmt.Errorf("STS client not initialized")
	}

	_, err := c.clients.STS.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("AWS connection test failed: %w", err)
	}

	return nil
}

// Close performs any necessary cleanup
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// AWS SDK v2 clients don't require explicit closing
	// but we can clear references
	c.clients = nil
	c.userIdentity = nil
	c.lambdaService = nil

	logger.Debug("AWS client closed", zap.String("profile", c.profile))
	return nil
}
