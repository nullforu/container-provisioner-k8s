package stack

import (
	"context"
	"fmt"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"smctf/internal/config"
)

func NewRepositoryFromConfig(ctx context.Context, cfg config.StackConfig) (RepositoryClientAPI, error) {
	if cfg.UseMockRepository {
		return NewInMemoryRepository(0), nil
	}

	loadOpts := []func(*awscfg.LoadOptions) error{
		awscfg.WithRegion(cfg.AWSRegion),
	}

	if cfg.AWSProfile != "" {
		loadOpts = append(loadOpts, awscfg.WithSharedConfigProfile(cfg.AWSProfile))
	}

	awsConfig, err := awscfg.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	var client *dynamodb.Client

	if cfg.AWSEndpoint != "" {
		client = dynamodb.NewFromConfig(awsConfig, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(cfg.AWSEndpoint)
		})
	} else {
		client = dynamodb.NewFromConfig(awsConfig)
	}

	return NewDynamoRepository(
		client,
		cfg.DynamoTableName,
		cfg.DynamoConsistentRead,
		cfg.PortLockTTL,
	), nil
}

func NewKubernetesClientFromConfig(cfg config.StackConfig) (KubernetesClientAPI, error) {
	if cfg.UseMockKubernetes {
		return NewMockKubernetesClient(0), nil
	}

	return NewKubernetesClient(cfg)
}
