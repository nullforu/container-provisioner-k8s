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
	Dir               string
	FilePrefix        string
	DiscordWebhookURL string
	SlackWebhookURL   string
	MaxBodyBytes      int
	WebhookQueueSize  int
	WebhookTimeout    time.Duration
	WebhookBatchSize  int
	WebhookBatchWait  time.Duration
	WebhookMaxChars   int
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

	logWebhookQueueSize, err := getEnvInt("LOG_WEBHOOK_QUEUE_SIZE", 1000)
	if err != nil {
		errs = append(errs, err)
	}

	logWebhookTimeout, err := getDuration("LOG_WEBHOOK_TIMEOUT", 5*time.Second)
	if err != nil {
		errs = append(errs, err)
	}

	logWebhookBatchSize, err := getEnvInt("LOG_WEBHOOK_BATCH_SIZE", 20)
	if err != nil {
		errs = append(errs, err)
	}

	logWebhookBatchWait, err := getDuration("LOG_WEBHOOK_BATCH_WAIT", 2*time.Second)
	if err != nil {
		errs = append(errs, err)
	}

	logWebhookMaxChars, err := getEnvInt("LOG_WEBHOOK_MAX_CHARS", 1800)
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
			Dir:               logDir,
			FilePrefix:        logPrefix,
			DiscordWebhookURL: getEnv("LOG_DISCORD_WEBHOOK_URL", ""),
			SlackWebhookURL:   getEnv("LOG_SLACK_WEBHOOK_URL", ""),
			MaxBodyBytes:      logMaxBodyBytes,
			WebhookQueueSize:  logWebhookQueueSize,
			WebhookTimeout:    logWebhookTimeout,
			WebhookBatchSize:  logWebhookBatchSize,
			WebhookBatchWait:  logWebhookBatchWait,
			WebhookMaxChars:   logWebhookMaxChars,
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

	if cfg.Logging.WebhookQueueSize <= 0 {
		errs = append(errs, errors.New("LOG_WEBHOOK_QUEUE_SIZE must be positive"))
	}

	if cfg.Logging.WebhookTimeout <= 0 {
		errs = append(errs, errors.New("LOG_WEBHOOK_TIMEOUT must be positive"))
	}

	if cfg.Logging.WebhookBatchSize <= 0 {
		errs = append(errs, errors.New("LOG_WEBHOOK_BATCH_SIZE must be positive"))
	}

	if cfg.Logging.WebhookBatchWait <= 0 {
		errs = append(errs, errors.New("LOG_WEBHOOK_BATCH_WAIT must be positive"))
	}

	if cfg.Logging.WebhookMaxChars <= 0 {
		errs = append(errs, errors.New("LOG_WEBHOOK_MAX_CHARS must be positive"))
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
	cfg.Logging.DiscordWebhookURL = redact(cfg.Logging.DiscordWebhookURL)
	cfg.Logging.SlackWebhookURL = redact(cfg.Logging.SlackWebhookURL)

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

func FormatForLog(cfg Config) string {
	cfg = Redact(cfg)
	var b strings.Builder
	fmt.Fprintf(&b, "AppEnv=%s\n", cfg.AppEnv)
	fmt.Fprintf(&b, "HTTPAddr=%s\n", cfg.HTTPAddr)
	fmt.Fprintf(&b, "ShutdownTimeout=%s\n", cfg.ShutdownTimeout)
	fmt.Fprintln(&b, "Logging:")
	fmt.Fprintf(&b, "  Dir=%s\n", cfg.Logging.Dir)
	fmt.Fprintf(&b, "  FilePrefix=%s\n", cfg.Logging.FilePrefix)
	fmt.Fprintf(&b, "  DiscordWebhookURL=%s\n", cfg.Logging.DiscordWebhookURL)
	fmt.Fprintf(&b, "  SlackWebhookURL=%s\n", cfg.Logging.SlackWebhookURL)
	fmt.Fprintf(&b, "  MaxBodyBytes=%d\n", cfg.Logging.MaxBodyBytes)
	fmt.Fprintf(&b, "  WebhookQueueSize=%d\n", cfg.Logging.WebhookQueueSize)
	fmt.Fprintf(&b, "  WebhookTimeout=%s\n", cfg.Logging.WebhookTimeout)
	fmt.Fprintf(&b, "  WebhookBatchSize=%d\n", cfg.Logging.WebhookBatchSize)
	fmt.Fprintf(&b, "  WebhookBatchWait=%s\n", cfg.Logging.WebhookBatchWait)
	fmt.Fprintf(&b, "  WebhookMaxChars=%d\n", cfg.Logging.WebhookMaxChars)
	fmt.Fprintln(&b, "Stack:")
	fmt.Fprintf(&b, "  Namespace=%s\n", cfg.Stack.Namespace)
	fmt.Fprintf(&b, "  StackTTL=%s\n", cfg.Stack.StackTTL)
	fmt.Fprintf(&b, "  SchedulerInterval=%s\n", cfg.Stack.SchedulerInterval)
	fmt.Fprintf(&b, "  NodePortRange=%d-%d\n", cfg.Stack.NodePortMin, cfg.Stack.NodePortMax)
	fmt.Fprintf(&b, "  PortLockTTL=%s\n", cfg.Stack.PortLockTTL)
	fmt.Fprintf(&b, "  DynamoTableName=%s\n", cfg.Stack.DynamoTableName)
	fmt.Fprintf(&b, "  AWSRegion=%s\n", cfg.Stack.AWSRegion)
	fmt.Fprintf(&b, "  AWSEndpoint=%s\n", cfg.Stack.AWSEndpoint)
	fmt.Fprintf(&b, "  DynamoConsistentRead=%t\n", cfg.Stack.DynamoConsistentRead)
	fmt.Fprintf(&b, "  UseMockRepository=%t\n", cfg.Stack.UseMockRepository)
	fmt.Fprintf(&b, "  KubeConfigPath=%s\n", cfg.Stack.KubeConfigPath)
	fmt.Fprintf(&b, "  KubeContext=%s\n", cfg.Stack.KubeContext)
	fmt.Fprintf(&b, "  K8sQPS=%.2f\n", cfg.Stack.K8sQPS)
	fmt.Fprintf(&b, "  K8sBurst=%d\n", cfg.Stack.K8sBurst)
	fmt.Fprintf(&b, "  SchedulingTimeout=%s\n", cfg.Stack.SchedulingTimeout)
	fmt.Fprintf(&b, "  UseMockKubernetes=%t\n", cfg.Stack.UseMockKubernetes)
	fmt.Fprintf(&b, "  RequireIngressNetworkPolicy=%t\n", cfg.Stack.RequireIngressNP)
	fmt.Fprintf(&b, "  StackNodeRole=%s\n", cfg.Stack.StackNodeRole)

	return b.String()
}
