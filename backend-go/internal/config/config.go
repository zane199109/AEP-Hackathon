package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	DB       DBConfig       `mapstructure:"db"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Chain    ChainConfig    `mapstructure:"chain"`
	CAW      CAWConfig      `mapstructure:"caw"`
	FluxA    FluxAConfig    `mapstructure:"fluxa"`
	OpenAI   OpenAIConfig   `mapstructure:"openai"`
	IPFS     IPFSConfig     `mapstructure:"ipfs"`
	Log      LogConfig      `mapstructure:"log"`
}

type ServerConfig struct {
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

type DBConfig struct {
	DSN     string `mapstructure:"dsn"`
	MaxOpen int    `mapstructure:"max_open"`
	MaxIdle int    `mapstructure:"max_idle"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type ChainConfig struct {
	RPCURL           string `mapstructure:"rpc_url"`
	ContractAddr     string `mapstructure:"contract_addr"`
	PrivateKey       string `mapstructure:"private_key"`
	ReputationAddr   string `mapstructure:"reputation_addr"`
}

type CAWConfig struct {
	APIKey    string        `mapstructure:"api_key"`
	WalletID  string        `mapstructure:"wallet_id"`
	Sandbox   bool          `mapstructure:"sandbox"`
	DevMode   bool          `mapstructure:"dev_mode"`
	Timeout   time.Duration `mapstructure:"timeout"`
	Retry     RetryConfig   `mapstructure:"retry"`
}

type FluxAConfig struct {
	Enabled bool          `mapstructure:"enabled"`
	BaseURL string        `mapstructure:"base_url"`
	Timeout time.Duration `mapstructure:"timeout"`
}

type OpenAIConfig struct {
	APIKey  string        `mapstructure:"api_key"`
	Model   string        `mapstructure:"model"`
	BaseURL string        `mapstructure:"base_url"`
	Timeout time.Duration `mapstructure:"timeout"`
}

type IPFSConfig struct {
	PinataJWT string      `mapstructure:"pinata_jwt"`
	Timeout   time.Duration `mapstructure:"timeout"`
	Retry     RetryConfig   `mapstructure:"retry"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type RetryConfig struct {
	MaxAttempts int           `mapstructure:"max_attempts"`
	BaseDelay   time.Duration `mapstructure:"base_delay"`
}

// LogLevel returns the zapcore.Level for the configured log level.
func (l LogConfig) LogLevel() zapcore.Level {
	switch l.Level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// env reads a value from env var, falling back to config value if empty.
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Load reads config from file, with env var overrides for secrets.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Explicit env var overrides (Viper's AutomaticEnv doesn't override empty strings)
	cfg.CAW.APIKey = env("CAW_API_KEY", cfg.CAW.APIKey)
	cfg.CAW.WalletID = env("CAW_WALLET_ID", cfg.CAW.WalletID)
	cfg.Chain.RPCURL = env("BASE_SEPOLIA_RPC", cfg.Chain.RPCURL)
	cfg.Chain.PrivateKey = env("PRIVATE_KEY", cfg.Chain.PrivateKey)
	cfg.OpenAI.APIKey = env("OPENAI_API_KEY", cfg.OpenAI.APIKey)
	cfg.IPFS.PinataJWT = env("PINATA_JWT", cfg.IPFS.PinataJWT)
	cfg.Chain.ReputationAddr = env("REPUTATION_CONTRACT_ADDR", cfg.Chain.ReputationAddr)

	// Debug: warn if key values are still empty
	if cfg.OpenAI.APIKey == "" {
		fmt.Fprintf(os.Stderr, "[config] WARNING: OPENAI_API_KEY not set — LLM evaluation will fail\n")
	}
	if cfg.CAW.APIKey == "" {
		fmt.Fprintf(os.Stderr, "[config] WARNING: CAW_API_KEY not set — CAW operations will fail\n")
	}
	if cfg.CAW.WalletID == "" {
		fmt.Fprintf(os.Stderr, "[config] WARNING: CAW_WALLET_ID not set — CAW operations will fail\n")
	}

	return &cfg, nil
}
