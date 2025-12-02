package aws

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"swiss-army-tui/pkg/logger"

	"go.uber.org/zap"
)

// Profile represents an AWS profile
type Profile struct {
	Name          string `json:"name"`
	Region        string `json:"region,omitempty"`
	Output        string `json:"output,omitempty"`
	Source        string `json:"source,omitempty"` // "config" or "credentials"
	RoleARN       string `json:"role_arn,omitempty"`
	SourceProfile string `json:"source_profile,omitempty"`
}

// ProfileManager manages AWS profiles
type ProfileManager struct {
	configPath      string
	credentialsPath string
	profiles        map[string]*Profile
}

// NewProfileManager creates a new profile manager
func NewProfileManager(configPath, credentialsPath string) *ProfileManager {
	return &ProfileManager{
		configPath:      configPath,
		credentialsPath: credentialsPath,
		profiles:        make(map[string]*Profile),
	}
}

// LoadProfiles loads all AWS profiles from config and credentials files
func (pm *ProfileManager) LoadProfiles() error {
	logger.Debug("Loading AWS profiles",
		zap.String("config_path", pm.configPath),
		zap.String("credentials_path", pm.credentialsPath))

	// Clear existing profiles
	pm.profiles = make(map[string]*Profile)

	// Load from config file
	if err := pm.loadFromConfigFile(); err != nil {
		logger.Warn("Failed to load from config file", zap.Error(err))
	}

	// Load from credentials file
	if err := pm.loadFromCredentialsFile(); err != nil {
		logger.Warn("Failed to load from credentials file", zap.Error(err))
	}

	logger.Info("Loaded AWS profiles", zap.Int("count", len(pm.profiles)))
	return nil
}

// GetProfiles returns all loaded profiles
func (pm *ProfileManager) GetProfiles() map[string]*Profile {
	return pm.profiles
}

// GetProfile returns a specific profile by name
func (pm *ProfileManager) GetProfile(name string) (*Profile, bool) {
	profile, exists := pm.profiles[name]
	return profile, exists
}

// GetProfileNames returns a sorted list of profile names
func (pm *ProfileManager) GetProfileNames() []string {
	names := make([]string, 0, len(pm.profiles))
	for name := range pm.profiles {
		names = append(names, name)
	}
	return names
}

// ValidateProfile checks if a profile exists and is valid
func (pm *ProfileManager) ValidateProfile(name string) error {
	profile, exists := pm.profiles[name]
	if !exists {
		return fmt.Errorf("profile '%s' not found", name)
	}

	if profile.Name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}

	// Additional validation can be added here
	return nil
}

// loadFromConfigFile loads profiles from AWS config file
func (pm *ProfileManager) loadFromConfigFile() error {
	if _, err := os.Stat(pm.configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s", pm.configPath)
	}

	file, err := os.Open(pm.configPath)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var currentProfile *Profile
	var currentSection string

	// Regex patterns
	profilePattern := regexp.MustCompile(`^\[profile\s+(.+)\]$`)
	defaultPattern := regexp.MustCompile(`^\[default\]$`)
	keyValuePattern := regexp.MustCompile(`^(\w+)\s*=\s*(.+)$`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for profile section
		if matches := profilePattern.FindStringSubmatch(line); matches != nil {
			profileName := strings.TrimSpace(matches[1])
			currentProfile = &Profile{
				Name:   profileName,
				Source: "config",
			}
			pm.profiles[profileName] = currentProfile
			currentSection = "profile"
			continue
		}

		// Check for default section
		if defaultPattern.MatchString(line) {
			currentProfile = &Profile{
				Name:   "default",
				Source: "config",
			}
			pm.profiles["default"] = currentProfile
			currentSection = "profile"
			continue
		}

		// Parse key-value pairs
		if currentProfile != nil && currentSection == "profile" {
			if matches := keyValuePattern.FindStringSubmatch(line); matches != nil {
				key := strings.TrimSpace(matches[1])
				value := strings.TrimSpace(matches[2])

				switch strings.ToLower(key) {
				case "region":
					currentProfile.Region = value
				case "output":
					currentProfile.Output = value
				case "role_arn":
					currentProfile.RoleARN = value
				case "source_profile":
					currentProfile.SourceProfile = value
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	return nil
}

// loadFromCredentialsFile loads profiles from AWS credentials file
func (pm *ProfileManager) loadFromCredentialsFile() error {
	if _, err := os.Stat(pm.credentialsPath); os.IsNotExist(err) {
		return fmt.Errorf("credentials file does not exist: %s", pm.credentialsPath)
	}

	file, err := os.Open(pm.credentialsPath)
	if err != nil {
		return fmt.Errorf("failed to open credentials file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var currentProfileName string

	// Regex patterns
	profilePattern := regexp.MustCompile(`^\[(.+)\]$`)
	keyValuePattern := regexp.MustCompile(`^(\w+)\s*=\s*(.+)$`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for profile section
		if matches := profilePattern.FindStringSubmatch(line); matches != nil {
			currentProfileName = strings.TrimSpace(matches[1])

			// If profile doesn't exist from config, create it
			if _, exists := pm.profiles[currentProfileName]; !exists {
				pm.profiles[currentProfileName] = &Profile{
					Name:   currentProfileName,
					Source: "credentials",
				}
			}
			continue
		}

		// Parse key-value pairs (we don't store sensitive data, just mark as valid)
		if currentProfileName != "" {
			if matches := keyValuePattern.FindStringSubmatch(line); matches != nil {
				key := strings.TrimSpace(matches[1])

				// Just validate that it has credentials
				if strings.ToLower(key) == "aws_access_key_id" ||
					strings.ToLower(key) == "aws_secret_access_key" {
					if profile, exists := pm.profiles[currentProfileName]; exists {
						if profile.Source == "config" {
							profile.Source = "both"
						}
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading credentials file: %w", err)
	}

	return nil
}

// GetDefaultConfigPath returns the default AWS config file path
func GetDefaultConfigPath() string {
	if configPath := os.Getenv("AWS_CONFIG_FILE"); configPath != "" {
		return configPath
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(homeDir, ".aws", "config")
}

// GetDefaultCredentialsPath returns the default AWS credentials file path
func GetDefaultCredentialsPath() string {
	if credPath := os.Getenv("AWS_SHARED_CREDENTIALS_FILE"); credPath != "" {
		return credPath
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(homeDir, ".aws", "credentials")
}

// CreateAWSConfigIfNotExists creates basic AWS config structure if it doesn't exist
func CreateAWSConfigIfNotExists() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	awsDir := filepath.Join(homeDir, ".aws")
	if err := os.MkdirAll(awsDir, 0755); err != nil {
		return fmt.Errorf("failed to create .aws directory: %w", err)
	}

	// Create basic config file if it doesn't exist
	configPath := filepath.Join(awsDir, "config")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultConfig := `[default]
region = us-east-1
output = json
`
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
			return fmt.Errorf("failed to create default config file: %w", err)
		}
	}

	// Create basic credentials file if it doesn't exist
	credPath := filepath.Join(awsDir, "credentials")
	if _, err := os.Stat(credPath); os.IsNotExist(err) {
		defaultCreds := `[default]
# aws_access_key_id = your_access_key_here
# aws_secret_access_key = your_secret_key_here
`
		if err := os.WriteFile(credPath, []byte(defaultCreds), 0644); err != nil {
			return fmt.Errorf("failed to create default credentials file: %w", err)
		}
	}

	return nil
}
