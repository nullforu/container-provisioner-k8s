package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv          string
	HTTPAddr        string
	ShutdownTimeout time.Duration

	Logging LoggingConfig
	APIKey  APIKeyConfig
	Stack   StackConfig
}

type LoggingConfig struct {
	Dir          string
	FilePrefix   string
	MaxBodyBytes int
}

type APIKeyConfig struct {
	Enabled bool
	Value   string
}

type StackConfig struct {
	Namespace         string
	StackTTL          time.Duration
	SchedulerInterval time.Duration
	NodePortMin       int
	NodePortMax       int
	PortLockTTL       time.Duration

	DynamoTableName      string
	AWSRegion            string
	AWSEndpoint          string
	DynamoConsistentRead bool
	UseMockRepository    bool

	KubeConfigPath    string
	KubeContext       string
	K8sQPS            float64
	K8sBurst          int
	SchedulingTimeout time.Duration
	UseMockKubernetes bool
	RequireIngressNP  bool
	StackNodeRole     string
}

func Load() (Config, error) {
	var errs []error

	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("load .env: %w", err))
	}

	appEnv := getEnv("APP_ENV", "local")
	httpAddr := getEnv("HTTP_ADDR", ":8081")
	shutdownTimeout, err := getDuration("SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		errs = append(errs, err)
	}

	logDir := getEnv("LOG_DIR", "logs")
	logPrefix := getEnv("LOG_FILE_PREFIX", "app")
	logMaxBodyBytes, err := getEnvInt("LOG_MAX_BODY_BYTES", 1024*1024)
	if err != nil {
		errs = append(errs, err)
	}

	apiKeyEnabled, err := getEnvBool("API_KEY_ENABLED", true)
	if err != nil {
		errs = append(errs, err)
	}

	apiKeyValue := getEnv("API_KEY", "")

	stackTTL, err := getDuration("STACK_TTL", 2*time.Hour)
	if err != nil {
		errs = append(errs, err)
	}

	schedulerInterval, err := getDuration("STACK_SCHEDULER_INTERVAL", 10*time.Second)
	if err != nil {
		errs = append(errs, err)
	}

	nodePortMin, err := getEnvInt("STACK_NODEPORT_MIN", 31001)
	if err != nil {
		errs = append(errs, err)
	}

	nodePortMax, err := getEnvInt("STACK_NODEPORT_MAX", 32767)
	if err != nil {
		errs = append(errs, err)
	}

	portLockTTL, err := getDuration("STACK_PORT_LOCK_TTL", 30*time.Second)
	if err != nil {
		errs = append(errs, err)
	}

	useMockK8s, err := getEnvBool("K8S_USE_MOCK", false)
	if err != nil {
		errs = append(errs, err)
	}

	useMockRepo, err := getEnvBool("DDB_USE_MOCK", false)
	if err != nil {
		errs = append(errs, err)
	}

	dynamoConsistentRead, err := getEnvBool("DDB_CONSISTENT_READ", true)
	if err != nil {
		errs = append(errs, err)
	}

	k8sQPS, err := getEnvFloat64("K8S_CLIENT_QPS", 20)
	if err != nil {
		errs = append(errs, err)
	}

	k8sBurst, err := getEnvInt("K8S_CLIENT_BURST", 40)
	if err != nil {
		errs = append(errs, err)
	}
	schedulingTimeout, err := getDuration("STACK_SCHEDULING_TIMEOUT", 20*time.Second)
	if err != nil {
		errs = append(errs, err)
	}

	requireIngressNP, err := getEnvBool("STACK_REQUIRE_INGRESS_NETWORK_POLICY", true)
	if err != nil {
		errs = append(errs, err)
	}
	stackNodeRole := getEnv("STACK_NODE_ROLE", "stack")

	cfg := Config{
		AppEnv:          appEnv,
		HTTPAddr:        httpAddr,
		ShutdownTimeout: shutdownTimeout,
		Logging: LoggingConfig{
			Dir:          logDir,
			FilePrefix:   logPrefix,
			MaxBodyBytes: logMaxBodyBytes,
		},
		APIKey: APIKeyConfig{
			Enabled: apiKeyEnabled,
			Value:   apiKeyValue,
		},
		Stack: StackConfig{
			Namespace:            getEnv("STACK_NAMESPACE", "stacks"),
			StackTTL:             stackTTL,
			SchedulerInterval:    schedulerInterval,
			NodePortMin:          nodePortMin,
			NodePortMax:          nodePortMax,
			PortLockTTL:          portLockTTL,
			DynamoTableName:      getEnv("DDB_STACK_TABLE", "smctf-stacks"),
			AWSRegion:            getEnv("AWS_REGION", "us-east-1"),
			AWSEndpoint:          getEnv("AWS_ENDPOINT", ""),
			DynamoConsistentRead: dynamoConsistentRead,
			UseMockRepository:    useMockRepo,
			KubeConfigPath:       getEnv("K8S_KUBECONFIG", ""),
			KubeContext:          getEnv("K8S_CONTEXT", ""),
			K8sQPS:               k8sQPS,
			K8sBurst:             k8sBurst,
			SchedulingTimeout:    schedulingTimeout,
			UseMockKubernetes:    useMockK8s,
			RequireIngressNP:     requireIngressNP,
			StackNodeRole:        stackNodeRole,
		},
	}

	if err := validateConfig(cfg); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}

	return cfg, nil
}

func getEnv(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}

	return v
}

func getEnvInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return def, fmt.Errorf("%s must be an integer", key)
	}

	return n, nil
}

func getEnvFloat64(key string, def float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}

	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def, fmt.Errorf("%s must be a number", key)
	}

	return f, nil
}

func getEnvBool(key string, def bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}

	b, err := strconv.ParseBool(v)
	if err != nil {
		return def, fmt.Errorf("%s must be a boolean", key)
	}

	return b, nil
}

func getDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}

	d, err := time.ParseDuration(v)
	if err != nil {
		return def, fmt.Errorf("%s must be a duration", key)
	}

	return d, nil
}

func getEnvCPUMilli(key, def string) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		v = def
	}

	milli, err := ParseCPUMilli(v)
	if err != nil {
		return 0, fmt.Errorf("%s invalid cpu: %w", key, err)
	}

	return milli, nil
}

func getEnvBytes(key, def string) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		v = def
	}

	b, err := ParseBytes(v)
	if err != nil {
		return 0, fmt.Errorf("%s invalid memory: %w", key, err)
	}

	return b, nil
}

func validateConfig(cfg Config) error {
	var errs []error

	if cfg.HTTPAddr == "" {
		errs = append(errs, errors.New("HTTP_ADDR must not be empty"))
	}

	if cfg.Logging.Dir == "" {
		errs = append(errs, errors.New("LOG_DIR must not be empty"))
	}

	if cfg.Logging.FilePrefix == "" {
		errs = append(errs, errors.New("LOG_FILE_PREFIX must not be empty"))
	}

	if cfg.Logging.MaxBodyBytes <= 0 {
		errs = append(errs, errors.New("LOG_MAX_BODY_BYTES must be positive"))
	}

	if cfg.APIKey.Enabled && strings.TrimSpace(cfg.APIKey.Value) == "" {
		errs = append(errs, errors.New("API_KEY must not be empty when API_KEY_ENABLED=true"))
	}

	if cfg.Stack.Namespace == "" {
		errs = append(errs, errors.New("STACK_NAMESPACE must not be empty"))
	}

	if cfg.Stack.StackTTL <= 0 {
		errs = append(errs, errors.New("STACK_TTL must be positive"))
	}

	if cfg.Stack.SchedulerInterval <= 0 {
		errs = append(errs, errors.New("STACK_SCHEDULER_INTERVAL must be positive"))
	}

	if cfg.Stack.NodePortMin < 1 || cfg.Stack.NodePortMax > 65535 || cfg.Stack.NodePortMin > cfg.Stack.NodePortMax {
		errs = append(errs, errors.New("STACK_NODEPORT range is invalid"))
	}

	if cfg.Stack.PortLockTTL <= 0 {
		errs = append(errs, errors.New("STACK_PORT_LOCK_TTL must be positive"))
	}

	if cfg.Stack.K8sQPS <= 0 {
		errs = append(errs, errors.New("K8S_CLIENT_QPS must be positive"))
	}

	if cfg.Stack.K8sBurst <= 0 {
		errs = append(errs, errors.New("K8S_CLIENT_BURST must be positive"))
	}

	if cfg.Stack.SchedulingTimeout <= 0 {
		errs = append(errs, errors.New("STACK_SCHEDULING_TIMEOUT must be positive"))
	}

	if cfg.Stack.RequireIngressNP && cfg.Stack.Namespace == "" {
		errs = append(errs, errors.New("STACK_REQUIRE_INGRESS_NETWORK_POLICY requires STACK_NAMESPACE"))
	}

	if cfg.Stack.StackNodeRole == "" {
		errs = append(errs, errors.New("STACK_NODE_ROLE must not be empty"))
	}

	if !cfg.Stack.UseMockRepository && cfg.Stack.DynamoTableName == "" {
		errs = append(errs, errors.New("DDB_STACK_TABLE must not be empty when DDB_USE_MOCK=false"))
	}

	if !cfg.Stack.UseMockRepository && cfg.Stack.AWSRegion == "" {
		errs = append(errs, errors.New("AWS_REGION must not be empty when DDB_USE_MOCK=false"))
	}

	if len(errs) == 0 {
		return nil
	}

	return errors.Join(errs...)
}

func Redact(cfg Config) Config {
	return cfg
}

func redact(value string) string {
	if value == "" {
		return ""
	}

	const (
		visiblePrefix = 2
		visibleSuffix = 2
	)
	if len(value) <= visiblePrefix+visibleSuffix {
		return "***"
	}

	return value[:visiblePrefix] + "***" + value[len(value)-visibleSuffix:]
}

func FormatForLog(cfg Config) map[string]any {
	cfg = Redact(cfg)

	return map[string]any{
		"app_env":          cfg.AppEnv,
		"http_addr":        cfg.HTTPAddr,
		"shutdown_timeout": seconds(cfg.ShutdownTimeout),
		"logging": map[string]any{
			"dir":            cfg.Logging.Dir,
			"file_prefix":    cfg.Logging.FilePrefix,
			"max_body_bytes": cfg.Logging.MaxBodyBytes,
		},
		"stack": map[string]any{
			"namespace":                      cfg.Stack.Namespace,
			"stack_ttl":                      seconds(cfg.Stack.StackTTL),
			"scheduler_interval":             seconds(cfg.Stack.SchedulerInterval),
			"node_port_min":                  cfg.Stack.NodePortMin,
			"node_port_max":                  cfg.Stack.NodePortMax,
			"port_lock_ttl":                  seconds(cfg.Stack.PortLockTTL),
			"dynamo_table_name":              cfg.Stack.DynamoTableName,
			"aws_region":                     cfg.Stack.AWSRegion,
			"aws_endpoint":                   cfg.Stack.AWSEndpoint,
			"dynamo_consistent_read":         cfg.Stack.DynamoConsistentRead,
			"use_mock_repository":            cfg.Stack.UseMockRepository,
			"kube_config_path":               cfg.Stack.KubeConfigPath,
			"kube_context":                   cfg.Stack.KubeContext,
			"k8s_qps":                        cfg.Stack.K8sQPS,
			"k8s_burst":                      cfg.Stack.K8sBurst,
			"scheduling_timeout":             seconds(cfg.Stack.SchedulingTimeout),
			"use_mock_kubernetes":            cfg.Stack.UseMockKubernetes,
			"require_ingress_network_policy": cfg.Stack.RequireIngressNP,
			"stack_node_role":                cfg.Stack.StackNodeRole,
		},
		"api_key": map[string]any{
			"enabled": cfg.APIKey.Enabled,
		},
	}
}

func seconds(d time.Duration) int64 {
	return int64(d.Seconds())
}
