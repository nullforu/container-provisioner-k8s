package config

import (
	"testing"
	"time"
)

func baseConfig() Config {
	return Config{
		AppEnv:          "test",
		HTTPAddr:        ":8081",
		ShutdownTimeout: 5 * time.Second,
		Logging: LoggingConfig{
			Dir:          "logs",
			FilePrefix:   "app",
			MaxBodyBytes: 1,
		},
		APIKey: APIKeyConfig{
			Enabled: false,
			Value:   "",
		},
		Stack: StackConfig{
			Namespace:         "stacks",
			StackTTL:          time.Second,
			SchedulerInterval: time.Second,
			NodePortMin:       1,
			NodePortMax:       2,
			PortLockTTL:       time.Second,
			LeaderElection: LeaderElectionConfig{
				Enabled:       true,
				Namespace:     "backend",
				LeaseName:     "container-provisioner",
				LeaseDuration: 15 * time.Second,
				RenewDeadline: 10 * time.Second,
				RetryPeriod:   2 * time.Second,
			},
			DynamoTableName:      "smctf-stacks",
			AWSRegion:            "us-east-1",
			DynamoConsistentRead: true,
			UseMockRepository:    false,
			K8sQPS:               1,
			K8sBurst:             1,
			SchedulingTimeout:    time.Second,
			RequireIngressNP:     false,
			StackNodeRole:        "stack",
		},
	}
}

func TestValidateConfigLeaderElectionOK(t *testing.T) {
	cfg := baseConfig()
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("expected leader election config to be valid, got: %v", err)
	}
}

func TestValidateConfigLeaderElectionDurations(t *testing.T) {
	cfg := baseConfig()
	cfg.Stack.LeaderElection.RenewDeadline = cfg.Stack.LeaderElection.LeaseDuration
	if err := validateConfig(cfg); err == nil {
		t.Fatalf("expected error when renew deadline >= lease duration")
	}

	cfg = baseConfig()
	cfg.Stack.LeaderElection.RetryPeriod = cfg.Stack.LeaderElection.RenewDeadline
	if err := validateConfig(cfg); err == nil {
		t.Fatalf("expected error when retry period >= renew deadline")
	}
}
