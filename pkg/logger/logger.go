package logger

import (
	"os"

	"go.uber.org/zap"
)

var (
	// Logger is the global logger instance
	Logger *zap.Logger
	// Sugar is the sugared logger instance for easier use
	Sugar *zap.SugaredLogger
)

// Config holds logger configuration
type Config struct {
	Level       string   `mapstructure:"level" yaml:"level"`
	Development bool     `mapstructure:"development" yaml:"development"`
	Encoding    string   `mapstructure:"encoding" yaml:"encoding"`
	OutputPaths []string `mapstructure:"output_paths" yaml:"output_paths"`
}

// DefaultConfig returns default logger configuration
func DefaultConfig() *Config {
	return &Config{
		Level:       "info",
		Development: false,
		Encoding:    "json",
		// Default to a log file instead of stdout to avoid interfering with the TUI screen
		OutputPaths: []string{"swiss-army-tui.log"},
	}
}

// Initialize sets up the global logger with the given configuration
func Initialize(cfg *Config) error {
	var zapCfg zap.Config

	if cfg.Development {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.Encoding = "console"
	} else {
		zapCfg = zap.NewProductionConfig()
		zapCfg.Encoding = cfg.Encoding
	}

	// Parse log level
	level, err := zap.ParseAtomicLevel(cfg.Level)
	if err != nil {
		return err
	}
	zapCfg.Level = level

	// Set output paths
	if len(cfg.OutputPaths) > 0 {
		zapCfg.OutputPaths = cfg.OutputPaths
	}

	// Build logger
	logger, err := zapCfg.Build()
	if err != nil {
		return err
	}

	Logger = logger
	Sugar = logger.Sugar()

	return nil
}

// InitializeDefault initializes the logger with default configuration
func InitializeDefault() error {
	cfg := DefaultConfig()
	cfg.Development = true // Default to development mode for TUI app
	return Initialize(cfg)
}

// Sync flushes any buffered log entries
func Sync() error {
	if Logger != nil {
		return Logger.Sync()
	}
	return nil
}

// Info logs an info message
func Info(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Info(msg, fields...)
	}
}

// Error logs an error message
func Error(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Error(msg, fields...)
	}
}

// Debug logs a debug message
func Debug(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Debug(msg, fields...)
	}
}

// Warn logs a warning message
func Warn(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Warn(msg, fields...)
	}
}

// Fatal logs a fatal message and exits
func Fatal(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Fatal(msg, fields...)
	}
	os.Exit(1)
}

// Infof logs an info message with formatting
func Infof(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Infof(template, args...)
	}
}

// Errorf logs an error message with formatting
func Errorf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Errorf(template, args...)
	}
}

// Debugf logs a debug message with formatting
func Debugf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Debugf(template, args...)
	}
}

// Warnf logs a warning message with formatting
func Warnf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Warnf(template, args...)
	}
}

// Fatalf logs a fatal message with formatting and exits
func Fatalf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Fatalf(template, args...)
	}
	os.Exit(1)
}
