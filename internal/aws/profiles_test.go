package aws

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileSSO(t *testing.T) {
	// Create a temporary directory for test config files
	tempDir, err := os.MkdirTemp("", "aws-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test config file with SSO profile
	configPath := filepath.Join(tempDir, "config")
	configContent := `[default]
region = us-east-1
output = json

[profile normal-profile]
region = us-west-2
output = json

[profile sso-profile]
sso_start_url = https://example.awsapps.com/start
sso_region = us-east-1
sso_account_id = 123456789012
sso_role_name = ExampleRole
region = us-west-2
output = json
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Create a test credentials file
	credPath := filepath.Join(tempDir, "credentials")
	credContent := `[default]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY

[normal-profile]
aws_access_key_id = AKIAI44QH8DHBEXAMPLE
aws_secret_access_key = je7MtGbClwBF/2Zp9Utk/h3yCo8nvbEXAMPLEKEY
`
	if err := os.WriteFile(credPath, []byte(credContent), 0644); err != nil {
		t.Fatalf("Failed to write credentials file: %v", err)
	}

	// Test profile loading
	pm := NewProfileManager(configPath, credPath)
	if err := pm.LoadProfiles(); err != nil {
		t.Fatalf("Failed to load profiles: %v", err)
	}

	// Test normal profile
	normalProfile, exists := pm.GetProfile("normal-profile")
	if !exists {
		t.Fatal("Normal profile not found")
	}
	if normalProfile.IsSSOProfileConfigured() {
		t.Error("Normal profile incorrectly identified as SSO profile")
	}

	// Test SSO profile
	ssoProfile, exists := pm.GetProfile("sso-profile")
	if !exists {
		t.Fatal("SSO profile not found")
	}
	if !ssoProfile.IsSSOProfileConfigured() {
		t.Error("SSO profile not identified correctly")
	}

	// Verify SSO fields
	if ssoProfile.SSOStartURL != "https://example.awsapps.com/start" {
		t.Errorf("Expected SSO start URL 'https://example.awsapps.com/start', got '%s'", ssoProfile.SSOStartURL)
	}
	if ssoProfile.SSORegion != "us-east-1" {
		t.Errorf("Expected SSO region 'us-east-1', got '%s'", ssoProfile.SSORegion)
	}
	if ssoProfile.SSOAccountID != "123456789012" {
		t.Errorf("Expected SSO account ID '123456789012', got '%s'", ssoProfile.SSOAccountID)
	}
	if ssoProfile.SSORoleName != "ExampleRole" {
		t.Errorf("Expected SSO role name 'ExampleRole', got '%s'", ssoProfile.SSORoleName)
	}

	// Test error message for SSO profile
	expectedErrMsg := "Profile 'sso-profile' is configured for AWS SSO. Please run 'aws sso login --profile sso-profile' to authenticate and try again."
	if errMsg := ssoProfile.GetSSOErrorMessage(); errMsg != expectedErrMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedErrMsg, errMsg)
	}

	// Test error message for non-SSO profile
	if errMsg := normalProfile.GetSSOErrorMessage(); errMsg != "" {
		t.Errorf("Expected empty error message for non-SSO profile, got '%s'", errMsg)
	}
}

func TestIsSSOError(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "SSO error",
			err:      &testError{msg: "SSO token expired"},
			expected: true,
		},
		{
			name:     "sso lowercase error",
			err:      &testError{msg: "sso login required"},
			expected: true,
		},
		{
			name:     "token error",
			err:      &testError{msg: "invalid token"},
			expected: true,
		},
		{
			name:     "expired error",
			err:      &testError{msg: "credentials expired"},
			expected: true,
		},
		{
			name:     "login error",
			err:      &testError{msg: "please login"},
			expected: true,
		},
		{
			name:     "authenticate error",
			err:      &testError{msg: "failed to authenticate"},
			expected: true,
		},
		{
			name:     "not authorized error",
			err:      &testError{msg: "not authorized"},
			expected: true,
		},
		{
			name:     "access denied error",
			err:      &testError{msg: "access denied"},
			expected: true,
		},
		{
			name:     "credentials error",
			err:      &testError{msg: "invalid credentials"},
			expected: true,
		},
		{
			name:     "non-SSO error",
			err:      &testError{msg: "network timeout"},
			expected: false,
		},
		{
			name:     "empty error",
			err:      &testError{msg: ""},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isSSOError(tc.err)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v for error '%s'", tc.expected, result, tc.err.Error())
			}
		})
	}
}

// testError is a simple error implementation for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
