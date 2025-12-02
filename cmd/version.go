package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	// Version information - these will be set by build flags
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long: `Print detailed version information including:
• Application version
• Build timestamp
• Git commit hash
• Go version and runtime information`,
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

// printVersion prints detailed version information
func printVersion() {
	fmt.Printf("Swiss Army TUI %s\n", version)
	fmt.Printf("Build time: %s\n", buildTime)
	fmt.Printf("Git commit: %s\n", gitCommit)
	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

// GetVersion returns the current version
func GetVersion() string {
	return version
}

// GetBuildInfo returns build information
func GetBuildInfo() map[string]string {
	return map[string]string{
		"version":   version,
		"buildTime": buildTime,
		"gitCommit": gitCommit,
		"goVersion": runtime.Version(),
		"platform":  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}
