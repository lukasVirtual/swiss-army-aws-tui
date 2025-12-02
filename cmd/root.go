package cmd

import (
	"fmt"
	"os"

	"swiss-army-tui/internal/aws"
	"swiss-army-tui/internal/config"
	"swiss-army-tui/internal/ui"
	"swiss-army-tui/pkg/logger"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	cfgFile     string
	verbose     bool
	configDir   string
	logLevel    string
	awsProfile  string
	awsRegion   string
	development bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "swiss-army-tui",
	Short: "DevOps Swiss Army Knife TUI",
	Long: `Swiss Army TUI is a comprehensive Terminal User Interface application
designed for DevOps engineers. It provides a beautiful, tabbed interface
to manage and monitor AWS resources, view logs, and configure settings.

Features:
• Multi-tab interface with AWS profile selection
• Real-time AWS resource monitoring (EC2, S3, RDS, Lambda, ECS)
• Integrated log viewer with filtering
• Configurable settings and themes
• Modern Go implementation with best practices`,
	RunE: runTUI,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.swiss-army-tui/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVar(&development, "dev", false, "enable development mode")

	// AWS flags
	rootCmd.PersistentFlags().StringVar(&awsProfile, "aws-profile", "", "AWS profile to use")
	rootCmd.PersistentFlags().StringVar(&awsRegion, "aws-region", "", "AWS region to use")

	// Bind flags to viper
	viper.BindPFlag("logger.level", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("logger.development", rootCmd.PersistentFlags().Lookup("dev"))
	viper.BindPFlag("aws.default_profile", rootCmd.PersistentFlags().Lookup("aws-profile"))
	viper.BindPFlag("aws.default_region", rootCmd.PersistentFlags().Lookup("aws-region"))
}

// initConfig reads in config file and ENV variables.
func initConfig() {
	// Initialize logger first with basic configuration
	if err := logger.InitializeDefault(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// Create default config directory if it doesn't exist
	if err := config.CreateDefaultConfigFile(); err != nil {
		logger.Warn("Failed to create default config", zap.Error(err))
	}

	// Create AWS config structure if it doesn't exist
	if err := aws.CreateAWSConfigIfNotExists(); err != nil {
		logger.Warn("Failed to create AWS config structure", zap.Error(err))
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Override with development mode if set
	if development {
		cfg.Logger.Development = true
		cfg.Logger.Level = "debug"
		cfg.App.Debug = true
	}

	// Override log level if verbose is set
	if verbose {
		cfg.Logger.Level = "debug"
	}

	// Reinitialize logger with loaded configuration
	if err := logger.Initialize(&cfg.Logger); err != nil {
		logger.Fatal("Failed to reinitialize logger", zap.Error(err))
	}

	logger.Info("Configuration loaded successfully",
		zap.String("app_name", cfg.App.Name),
		zap.String("version", cfg.App.Version),
		zap.String("log_level", cfg.Logger.Level),
		zap.Bool("development", cfg.Logger.Development))
}

// runTUI starts the TUI application
func runTUI(cmd *cobra.Command, args []string) error {
	logger.Info("Starting Swiss Army TUI application")

	// Get current configuration
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("configuration not loaded")
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Create and run TUI application
	app, err := ui.NewApp(cfg)
	if err != nil {
		return fmt.Errorf("failed to create TUI application: %w", err)
	}

	// Set up graceful shutdown
	defer func() {
		app.Quit()
		logger.Sync()
	}()

	// Run the application
	if err := app.Run(); err != nil {
		return fmt.Errorf("TUI application error: %w", err)
	}

	logger.Info("Swiss Army TUI application stopped")
	return nil
}
