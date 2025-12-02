package ui

import (
	"fmt"
	"strconv"
	"strings"

	"swiss-army-tui/internal/config"
	"swiss-army-tui/pkg/logger"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"go.uber.org/zap"
)

// SettingsTab represents the application settings tab
type SettingsTab struct {
	// Core components
	view   *tview.Flex
	config *config.Config

	// UI components
	form       *tview.Form
	infoPanel  *tview.TextView
	statusText *tview.TextView

	// State
	modified bool
}

// NewSettingsTab creates a new settings tab
func NewSettingsTab(cfg *config.Config) (*SettingsTab, error) {
	tab := &SettingsTab{
		config: cfg,
	}

	if err := tab.initializeUI(); err != nil {
		return nil, fmt.Errorf("failed to initialize settings tab UI: %w", err)
	}

	return tab, nil
}

// initializeUI initializes the UI components
func (st *SettingsTab) initializeUI() error {
	// Create status text first (needed by form field callbacks)
	st.statusText = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	st.statusText.SetBorder(true).SetTitle(" Status ").SetTitleAlign(tview.AlignLeft)

	// Create status text first (needed by form field callbacks)
	// Set initial status
	st.updateStatus("Settings loaded", "green")

	// Create settings form
	st.form = tview.NewForm()
	st.form.SetBorder(true).SetTitle(" Application Settings ").SetTitleAlign(tview.AlignLeft)

	// Add form fields based on configuration
	st.addFormFields()

	// Set form button handlers
	st.form.AddButton("Save", st.saveSettings)
	st.form.AddButton("Reset", st.resetSettings)
	st.form.AddButton("Export Config", st.exportConfig)

	// Add key bindings
	st.form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlS:
			st.saveSettings()
			return nil
		case tcell.KeyCtrlR:
			st.resetSettings()
			return nil
		}
		return event
	})

	// Create info panel
	st.infoPanel = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetScrollable(true)

	st.infoPanel.SetBorder(true).SetTitle(" Configuration Info ").SetTitleAlign(tview.AlignLeft)
	st.updateInfoPanel()

	// Set initial status
	st.updateStatus("Settings loaded", "green")

	// Create layout
	leftPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(st.form, 0, 1, true).
		AddItem(st.statusText, 5, 0, false)

	st.view = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(leftPanel, 0, 1, true).
		AddItem(st.infoPanel, 50, 0, false)

	return nil
}

// addFormFields adds configuration fields to the form
func (st *SettingsTab) addFormFields() {
	// Application settings
	st.form.AddTextView("Application", "", 0, 1, false, false)

	st.form.AddInputField("App Name", st.config.App.Name, 30, nil,
		func(text string) {
			st.config.App.Name = text
			st.markModified()
		})

	st.form.AddInputField("Version", st.config.App.Version, 15, nil,
		func(text string) {
			st.config.App.Version = text
			st.markModified()
		})

	st.form.AddInputField("Description", st.config.App.Description, 50, nil,
		func(text string) {
			st.config.App.Description = text
			st.markModified()
		})

	st.form.AddCheckbox("Debug Mode", st.config.App.Debug,
		func(checked bool) {
			st.config.App.Debug = checked
			st.markModified()
		})

	// AWS settings
	st.form.AddTextView("", "", 0, 1, false, false) // Spacer
	st.form.AddTextView("AWS Configuration", "", 0, 1, false, false)

	st.form.AddInputField("Default Profile", st.config.AWS.DefaultProfile, 20, nil,
		func(text string) {
			st.config.AWS.DefaultProfile = text
			st.markModified()
		})

	// Region dropdown
	regions := []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2",
		"eu-west-1", "eu-west-2", "eu-west-3", "eu-central-1", "eu-north-1",
		"ap-southeast-1", "ap-southeast-2", "ap-northeast-1", "ap-northeast-2", "ap-south-1",
		"ca-central-1", "sa-east-1", "af-south-1", "me-south-1",
		"ap-east-1", "ap-northeast-3", "eu-south-1",
	}

	currentRegionIndex := 0
	for i, region := range regions {
		if region == st.config.AWS.DefaultRegion {
			currentRegionIndex = i
			break
		}
	}

	st.form.AddDropDown("Default Region", regions, currentRegionIndex,
		func(option string, optionIndex int) {
			st.config.AWS.DefaultRegion = option
			st.markModified()
		})

	st.form.AddInputField("Config Path", st.config.AWS.ConfigPath, 50, nil,
		func(text string) {
			st.config.AWS.ConfigPath = text
			st.markModified()
		})

	st.form.AddInputField("Credentials Path", st.config.AWS.CredentialsPath, 50, nil,
		func(text string) {
			st.config.AWS.CredentialsPath = text
			st.markModified()
		})

	// UI settings
	st.form.AddTextView("", "", 0, 1, false, false) // Spacer
	st.form.AddTextView("User Interface", "", 0, 1, false, false)

	themes := []string{"dark", "light", "auto"}
	currentThemeIndex := 0
	for i, theme := range themes {
		if theme == st.config.UI.Theme {
			currentThemeIndex = i
			break
		}
	}

	st.form.AddDropDown("Theme", themes, currentThemeIndex,
		func(option string, optionIndex int) {
			st.config.UI.Theme = option
			st.markModified()
		})

	st.form.AddInputField("Refresh Interval (seconds)", strconv.Itoa(st.config.UI.RefreshInterval), 10,
		func(textToCheck string, lastChar rune) bool {
			_, err := strconv.Atoi(textToCheck)
			return err == nil || textToCheck == ""
		},
		func(text string) {
			if interval, err := strconv.Atoi(text); err == nil && interval > 0 {
				st.config.UI.RefreshInterval = interval
				st.markModified()
			}
		})

	st.form.AddCheckbox("Mouse Enabled", st.config.UI.MouseEnabled,
		func(checked bool) {
			st.config.UI.MouseEnabled = checked
			st.markModified()
		})

	borderStyles := []string{"rounded", "double", "single", "none"}
	currentBorderIndex := 0
	for i, style := range borderStyles {
		if style == st.config.UI.BorderStyle {
			currentBorderIndex = i
			break
		}
	}

	st.form.AddDropDown("Border Style", borderStyles, currentBorderIndex,
		func(option string, optionIndex int) {
			st.config.UI.BorderStyle = option
			st.markModified()
		})

	// Logger settings
	st.form.AddTextView("", "", 0, 1, false, false) // Spacer
	st.form.AddTextView("Logging", "", 0, 1, false, false)

	logLevels := []string{"debug", "info", "warn", "error"}
	currentLevelIndex := 0
	for i, level := range logLevels {
		if level == st.config.Logger.Level {
			currentLevelIndex = i
			break
		}
	}

	st.form.AddDropDown("Log Level", logLevels, currentLevelIndex,
		func(option string, optionIndex int) {
			st.config.Logger.Level = option
			st.markModified()
		})

	st.form.AddCheckbox("Development Mode", st.config.Logger.Development,
		func(checked bool) {
			st.config.Logger.Development = checked
			st.markModified()
		})

	encodings := []string{"json", "console"}
	currentEncodingIndex := 0
	for i, encoding := range encodings {
		if encoding == st.config.Logger.Encoding {
			currentEncodingIndex = i
			break
		}
	}

	st.form.AddDropDown("Log Encoding", encodings, currentEncodingIndex,
		func(option string, optionIndex int) {
			st.config.Logger.Encoding = option
			st.markModified()
		})
}

// markModified marks the configuration as modified
func (st *SettingsTab) markModified() {
	if st.statusText != nil {
		st.modified = true
		st.updateStatus("Configuration modified (unsaved)", "yellow")
		st.updateInfoPanel()
	}
}

// saveSettings saves the current settings
func (st *SettingsTab) saveSettings() {
	logger.Info("Saving configuration settings")

	// Validate configuration
	if err := st.config.Validate(); err != nil {
		st.updateStatus(fmt.Sprintf("Validation error: %s", err.Error()), "red")
		logger.Error("Configuration validation failed", zap.Error(err))
		return
	}

	// Save configuration
	if err := config.SaveConfig(); err != nil {
		st.updateStatus(fmt.Sprintf("Save failed: %s", err.Error()), "red")
		logger.Error("Failed to save configuration", zap.Error(err))
		return
	}

	// Reinitialize logger with new settings
	if err := logger.Initialize(&st.config.Logger); err != nil {
		st.updateStatus(fmt.Sprintf("Logger reinit failed: %s", err.Error()), "yellow")
		logger.Warn("Failed to reinitialize logger", zap.Error(err))
	}

	st.modified = false
	st.updateStatus("Configuration saved successfully", "green")
	st.updateInfoPanel()

	logger.Info("Configuration saved successfully")
}

// resetSettings resets settings to their original values
func (st *SettingsTab) resetSettings() {
	logger.Info("Resetting configuration settings")

	// Reload configuration from file/defaults
	newConfig, err := config.Load()
	if err != nil {
		st.updateStatus(fmt.Sprintf("Reset failed: %s", err.Error()), "red")
		logger.Error("Failed to reload configuration", zap.Error(err))
		return
	}

	st.config = newConfig
	st.modified = false

	// Recreate the form with reset values
	st.form.Clear(true)
	st.addFormFields()
	st.form.AddButton("Save", st.saveSettings)
	st.form.AddButton("Reset", st.resetSettings)
	st.form.AddButton("Export Config", st.exportConfig)

	st.updateStatus("Configuration reset to defaults", "blue")
	st.updateInfoPanel()

	logger.Info("Configuration reset successfully")
}

// exportConfig exports the current configuration
func (st *SettingsTab) exportConfig() {
	logger.Info("Exporting configuration")
	// This would typically open a file dialog or provide export options
	st.updateStatus("Export functionality coming soon", "blue")
}

// updateInfoPanel updates the configuration information panel
func (st *SettingsTab) updateInfoPanel() {
	// Guard against nil infoPanel during initialization
	if st.infoPanel == nil {
		return
	}

	info := fmt.Sprintf(`[yellow]Configuration Overview[-]

[blue]Application:[-]
• Name: %s
• Version: %s
• Debug: %t

[blue]AWS:[-]
• Default Profile: %s
• Default Region: %s
• Config Path: %s
• Credentials Path: %s

[blue]User Interface:[-]
• Theme: %s
• Refresh Interval: %ds
• Mouse Enabled: %t
• Border Style: %s

[blue]Logging:[-]
• Level: %s
• Development: %t
• Encoding: %s
• Output Paths: %s

[blue]Status:[-]
• Modified: %t

[blue]Shortcuts:[-]
• [white]Ctrl+S[-]: Save settings
• [white]Ctrl+R[-]: Reset settings
• [white]Tab[-]: Navigate form fields

[blue]Tips:[-]
• Changes are not applied until saved
• Some settings may require application restart
• Configuration is saved to ~/.swiss-army-tui/config.yaml
• Use Reset to discard unsaved changes`,
		st.config.App.Name,
		st.config.App.Version,
		st.config.App.Debug,
		st.config.AWS.DefaultProfile,
		st.config.AWS.DefaultRegion,
		st.config.AWS.ConfigPath,
		st.config.AWS.CredentialsPath,
		st.config.UI.Theme,
		st.config.UI.RefreshInterval,
		st.config.UI.MouseEnabled,
		st.config.UI.BorderStyle,
		st.config.Logger.Level,
		st.config.Logger.Development,
		st.config.Logger.Encoding,
		strings.Join(st.config.Logger.OutputPaths, ", "),
		st.modified)

	st.infoPanel.SetText(info)
}

// updateStatus updates the status display
func (st *SettingsTab) updateStatus(message, color string) {
	// Guard against nil statusText during initialization
	if st.statusText == nil {
		return
	}

	if st.statusText != nil {
		modifiedText := ""
		if st.modified {
			modifiedText = "\n[yellow]* Unsaved changes[-]"
		}

		statusText := fmt.Sprintf("[%s]%s[-]%s", color, message, modifiedText)
		st.statusText.SetText(statusText)
	}
}

// Refresh refreshes the settings tab
func (st *SettingsTab) Refresh() {
	logger.Debug("Refreshing settings tab")
	st.updateInfoPanel()
	st.updateStatus("Settings refreshed", "green")
}

// GetView returns the main view component
func (st *SettingsTab) GetView() tview.Primitive {
	return st.view
}

// IsModified returns whether the configuration has been modified
func (st *SettingsTab) IsModified() bool {
	return st.modified
}

// GetConfig returns the current configuration
func (st *SettingsTab) GetConfig() *config.Config {
	return st.config
}
