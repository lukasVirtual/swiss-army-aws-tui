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
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"go.uber.org/zap"
)

type ServiceClients struct {
	EC2    *ec2.Client
	S3     *s3.Client
	RDS    *rds.Client
	ECS    *ecs.Client
	Lambda *lambda.Client
	STS    *sts.Client
}

type Client struct {
	mu            sync.RWMutex
	config        aws.Config
	clients       *ServiceClients
	lambdaService *clients.LambdaService
	s3Service     *clients.S3Service
	ec2Service    *clients.EC2Service
	rdsService    *clients.RDSService
	profile       string
	region        string
	accountID     string
	userIdentity  *sts.GetCallerIdentityOutput
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

	c.clients = &ServiceClients{
		EC2:    ec2.NewFromConfig(c.config),
		S3:     s3.NewFromConfig(c.config),
		RDS:    rds.NewFromConfig(c.config),
		ECS:    ecs.NewFromConfig(c.config),
		Lambda: lambda.NewFromConfig(c.config),
		STS:    sts.NewFromConfig(c.config),
	}

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

	if c.clients != nil && c.clients.EC2 != nil {
		EC2Svc, err := clients.NewEC2Service(c.clients.EC2)
		if err != nil {
			logger.Debug("failed to initialize EC2 service wrapper", zap.Error(err))
		} else {
			c.ec2Service = EC2Svc
		}
	} else {
		logger.Debug("EC2 SDK client not initialized; skipping EC2 service wrapper")
	}

	if c.clients != nil && c.clients.RDS != nil {
		RDSSvc, err := clients.NewRDSService(c.clients.RDS)
		if err != nil {
			logger.Debug("failed to initialize RDS service wrapper", zap.Error(err))
		} else {
			c.rdsService = RDSSvc
		}
	} else {
		logger.Debug("RDS SDK client not initialized; skipping RDS service wrapper")
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
	svc := c.lambdaService
	c.mu.RUnlock()

	if svc == nil {
		return nil, fmt.Errorf("lambda service not initialized")
	}

	return svc.GetLambdaDetail(ctx)
}

func (c *Client) GetS3FunctionDetails(ctx context.Context) ([]clients.S3Details, error) {
	c.mu.RLock()
	svc := c.s3Service
	c.mu.RUnlock()

	if svc == nil {
		return nil, fmt.Errorf("S3 service not initialized")
	}

	return svc.GetS3Detail(ctx)
}

func (c *Client) GetEC2FunctionDetails(ctx context.Context) ([]types.Instance, error) {
	c.mu.RLock()
	svc := c.ec2Service
	c.mu.RUnlock()

	if svc == nil {
		return nil, fmt.Errorf("EC2 service not initialized")
	}

	return svc.GetEC2Detail(ctx)
}

// GetRDSFunctionDetails retrieves details of all RDS instances
func (c *Client) GetRDSFunctionDetails(ctx context.Context) ([]clients.RDSDetails, error) {
	c.mu.RLock()
	svc := c.rdsService
	c.mu.RUnlock()

	if svc == nil {
		return nil, fmt.Errorf("RDS service not initialized")
	}

	return svc.GetRDSDetail(ctx)
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
	c.lambdaService = nil
	c.s3Service = nil
	c.ec2Service = nil
	c.rdsService = nil

	logger.Debug("AWS client closed", zap.String("profile", c.profile))
	return nil
}
