// Package config loads configuration from config/default.toml, then
// config/<APP_ENV>.toml, then APP_* environment variables, each overriding the
// last.
package config

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// EnvPrefix namespaces the environment variables this service reads.
const EnvPrefix = "APP"

// Config is the fully resolved configuration for the process.
type Config struct {
	App      AppConfig      `mapstructure:"app"`
	Log      LogConfig      `mapstructure:"log"`
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Storage  StorageConfig  `mapstructure:"storage"`
	OCR      OCRConfig      `mapstructure:"ocr"`
}

// AppConfig carries service identity.
type AppConfig struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"`
}

// LogConfig controls the structured logger.
type LogConfig struct {
	// Level is one of debug, info, warn, error.
	Level string `mapstructure:"level"`
	// Format is either json or text.
	Format string `mapstructure:"format"`
}

// ServerConfig holds the listener addresses and the shutdown budget.
type ServerConfig struct {
	// GRPC serves native gRPC traffic.
	GRPC string `mapstructure:"grpc"`
	// HTTP serves REST/JSON, translated to gRPC by grpc-gateway.
	HTTP string `mapstructure:"http"`
	// ShutdownTimeout bounds how long in-flight requests may take to drain.
	ShutdownTimeout time.Duration `mapstructure:"shutdownTimeout"`
}

// DatabaseConfig locates the SQLite file.
type DatabaseConfig struct {
	Path string `mapstructure:"path"`
	// Debug logs every statement gorm executes.
	Debug bool `mapstructure:"debug"`
}

// StorageConfig controls where uploaded files go.
type StorageConfig struct {
	UploadDir      string `mapstructure:"uploadDir"`
	MaxUploadBytes int64  `mapstructure:"maxUploadBytes"`
}

// OCRConfig selects the OCR provider.
type OCRConfig struct {
	// Provider names the implementation; only "stub" exists.
	Provider string `mapstructure:"provider"`
	// FixturesDir is where the stub looks up text for non-text uploads.
	FixturesDir string `mapstructure:"fixturesDir"`
}

// Configurations are cached per environment.
var (
	mu          sync.Mutex
	configCache = map[string]*Config{}
	loadErrors  = map[string]error{}
)

// Get returns the configuration for env, loading it on first request and
// caching it thereafter. It is safe for concurrent use.
func Get(env string) (*Config, error) {
	if env == "" {
		env = "dev"
	}

	mu.Lock()
	defer mu.Unlock()

	if cfg, ok := configCache[env]; ok {
		return cfg, loadErrors[env]
	}

	cfg, err := load(env)
	configCache[env] = cfg
	loadErrors[env] = err

	return cfg, err
}

// load reads default.toml, merges <env>.toml over it, and lets environment
// variables win over both.
func load(env string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("toml")
	v.AddConfigPath("config")
	v.AddConfigPath("../config")
	v.AddConfigPath("../../config")
	v.AddConfigPath("../../../config")

	v.SetConfigName("default")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("config: reading default.toml: %w", err)
	}

	v.SetConfigName(env)
	if err := v.MergeInConfig(); err != nil {
		// A missing per-environment file is not fatal.
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("config: merging %s.toml: %w", env, err)
		}
	}

	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshalling: %w", err)
	}
	cfg.App.Env = env

	return &cfg, nil
}
