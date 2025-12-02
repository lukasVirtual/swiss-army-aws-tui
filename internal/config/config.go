package config

import (
	"fmt"
	"os"
	"path/filepath"

	"swiss-army-tui/pkg/logger"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	App    AppConfig     `mapstructure:"app" yaml:"app"`
	AWS    AWSConfig     `mapstructure:"aws" yaml:"aws"`
	UI     UIConfig      `mapstructure:"ui" yaml:"ui"`
	Logger logger.Config `mapstructure:"logger" yaml:"logger"`
}

// AppConfig holds general application configuration
type AppConfig struct {
	Name        string `mapstructure:"name" yaml:"name"`
	Version     string `mapstructure:"version" yaml:"version"`
	Description string `mapstructure:"description" yaml:"description"`
	Debug       bool   `mapstructure:"debug" yaml:"debug"`
}

// AWSConfig holds AWS-related configuration
type AWSConfig struct {
	DefaultProfile  string            `mapstructure:"default_profile" yaml:"default_profile"`
	DefaultRegion   string            `mapstructure:"default_region" yaml:"default_region"`
	Profiles        map[string]string `mapstructure:"profiles" yaml:"profiles"`
	ConfigPath      string            `mapstructure:"config_path" yaml:"config_path"`
	CredentialsPath string            `mapstructure:"credentials_path" yaml:"credentials_path"`
}

// UIConfig holds UI-related configuration
type UIConfig struct {
	Theme           string `mapstructure:"theme" yaml:"theme"`
	RefreshInterval int    `mapstructure:"refresh_interval" yaml:"refresh_interval"`
	MouseEnabled    bool   `mapstructure:"mouse_enabled" yaml:"mouse_enabled"`
	BorderStyle     string `mapstructure:"border_style" yaml:"border_style"`
}

var globalConfig *Config

// Load loads the configuration from file or environment variables
func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	// Add config search paths
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath("$HOME/.swiss-army-tui")

	// Set environment variable prefix
	viper.SetEnvPrefix("SAT")
	viper.AutomaticEnv()

	// Set default values
	setDefaults()

	// Try to read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found, use defaults
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Set AWS config paths if not specified
	if cfg.AWS.ConfigPath == "" {
		homeDir, _ := os.UserHomeDir()
		cfg.AWS.ConfigPath = filepath.Join(homeDir, ".aws", "config")
	}
	if cfg.AWS.CredentialsPath == "" {
		homeDir, _ := os.UserHomeDir()
		cfg.AWS.CredentialsPath = filepath.Join(homeDir, ".aws", "credentials")
	}

	globalConfig = &cfg
	return &cfg, nil
}

// Get returns the global configuration
func Get() *Config {
	return globalConfig
}

// setDefaults sets default configuration values
func setDefaults() {
	// App defaults
	viper.SetDefault("app.name", "Swiss Army TUI")
	viper.SetDefault("app.version", "1.0.0")
	viper.SetDefault("app.description", "DevOps Swiss Army Knife TUI")
	viper.SetDefault("app.debug", false)

	// AWS defaults
	viper.SetDefault("aws.default_profile", "default")
	viper.SetDefault("aws.default_region", "us-east-1")
	viper.SetDefault("aws.profiles", map[string]string{})

	// UI defaults
	viper.SetDefault("ui.theme", "dark")
	viper.SetDefault("ui.refresh_interval", 30)
	viper.SetDefault("ui.mouse_enabled", true)
	viper.SetDefault("ui.border_style", "rounded")

	// Logger defaults
	viper.SetDefault("logger.level", "info")
	viper.SetDefault("logger.development", true)
	viper.SetDefault("logger.encoding", "console")
	// Log to a file by default so log output does not interfere with the TUI screen
	viper.SetDefault("logger.output_paths", []string{"swiss-army-tui.log"})
}

// CreateDefaultConfigFile creates a default configuration file
func CreateDefaultConfigFile() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".swiss-army-tui")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configFile := filepath.Join(configDir, "config.yaml")

	// Check if config file already exists
	if _, err := os.Stat(configFile); err == nil {
		return nil // File already exists
	}

	defaultConfig := `app:
  name: "Swiss Army TUI"
  version: "1.0.0"
  description: "DevOps Swiss Army Knife TUI"
  debug: false

aws:
  default_profile: "default"
  default_region: "us-east-1"
  profiles: {}

ui:
  theme: "dark"
  refresh_interval: 30
  mouse_enabled: true
  border_style: "rounded"

logger:
  level: "info"
  development: true
  encoding: "console"
  output_paths:
    - "swiss-army-tui.log"
`

	if err := os.WriteFile(configFile, []byte(defaultConfig), 0644); err != nil {
		return fmt.Errorf("failed to write default config file: %w", err)
	}

	return nil
}

// SaveConfig saves the current configuration to file
func SaveConfig() error {
	if globalConfig == nil {
		return fmt.Errorf("no configuration loaded")
	}

	return viper.WriteConfig()
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.App.Name == "" {
		return fmt.Errorf("app name cannot be empty")
	}

	if c.UI.RefreshInterval <= 0 {
		return fmt.Errorf("refresh interval must be positive")
	}

	return nil
}
