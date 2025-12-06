package ui

import (
	"context"
	"fmt"
	"sync"
	"time"

	"swiss-army-tui/internal/aws"
	"swiss-army-tui/internal/config"
	"swiss-army-tui/pkg/logger"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"go.uber.org/zap"
)

// App represents the main TUI application
type App struct {
	// Core components
	app    *tview.Application
	config *config.Config

	// AWS components
	profileManager *aws.ProfileManager
	awsClient      *aws.Client

	// UI components
	pages        *tview.Pages
	tabs         *tview.TextView
	profileTab   *ProfileTab
	resourcesTab *ResourcesTab
	logsTab      *LogsTab
	settingsTab  *SettingsTab

	// State management
	currentTab int
	tabNames   []string
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc

	// Event handling
	eventChan chan Event
	stopChan  chan struct{}
}

// Event represents application events
type Event struct {
	Type string
	Data interface{}
}

const (
	EventProfileChanged = "profile_changed"
	EventRegionChanged  = "region_changed"
	EventRefresh        = "refresh"
	EventError          = "error"
	EventShowLambdaLogs = "show_lambda_logs"
)

// NewApp creates a new TUI application
func NewApp(cfg *config.Config) (*App, error) {
	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		app:        tview.NewApplication(),
		config:     cfg,
		tabNames:   []string{"Profiles", "Resources", "Logs", "Settings"},
		currentTab: 0,
		ctx:        ctx,
		cancel:     cancel,
		eventChan:  make(chan Event, 100),
		stopChan:   make(chan struct{}),
	}

	// Initialize profile manager
	app.profileManager = aws.NewProfileManager(cfg.AWS.ConfigPath, cfg.AWS.CredentialsPath)
	if err := app.profileManager.LoadProfiles(); err != nil {
		logger.Warn("Failed to load AWS profiles", zap.Error(err))
	}

	// Initialize UI components
	if err := app.initializeUI(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize UI: %w", err)
	}

	// Setup key bindings
	app.setupKeyBindings()

	// Start event handler
	go app.eventHandler()

	logger.Info("TUI application initialized successfully")
	return app, nil
}

// initializeUI initializes all UI components
func (app *App) initializeUI() error {
	var err error

	// Create main pages container
	app.pages = tview.NewPages()

	// Initialize tabs
	app.profileTab, err = NewProfileTab(app.profileManager, app.eventChan)
	if err != nil {
		return fmt.Errorf("failed to create profile tab: %w", err)
	}

	app.resourcesTab, err = NewResourcesTab(app.eventChan)
	if err != nil {
		return fmt.Errorf("failed to create resources tab: %w", err)
	}

	app.logsTab, err = NewLogsTab()
	if err != nil {
		return fmt.Errorf("failed to create logs tab: %w", err)
	}

	app.settingsTab, err = NewSettingsTab(app.config)
	if err != nil {
		return fmt.Errorf("failed to create settings tab: %w", err)
	}

	// Create tab navigation
	app.createTabNavigation()

	// Create main layout
	app.createMainLayout()

	// Add all pages with proper visibility - only first one visible
	app.pages.AddPage("profile", app.profileTab.GetView(), true, true)
	app.pages.AddPage("resources", app.resourcesTab.GetView(), true, false)
	app.pages.AddPage("logs", app.logsTab.GetView(), true, false)
	app.pages.AddPage("settings", app.settingsTab.GetView(), true, false)

	// Set initial tab
	app.switchTab(0)

	return nil
}

// createTabNavigation creates the tab navigation bar
func (app *App) createTabNavigation() {
	app.tabs = tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false).
		SetHighlightedFunc(func(added, removed, remaining []string) {
			// Handle tab selection
		})

	app.updateTabDisplay()
}

// updateTabDisplay updates the tab navigation display
func (app *App) updateTabDisplay() {
	tabText := ""
	for i, name := range app.tabNames {
		if i == app.currentTab {
			tabText += fmt.Sprintf(`[black:white:b]  %s  [""]`, name)
		} else {
			tabText += fmt.Sprintf(`[white:black]  %s  [""]`, name)
		}
		if i < len(app.tabNames)-1 {
			tabText += " "
		}
	}
	app.tabs.SetText(tabText)
}

// createMainLayout creates the main application layout
func (app *App) createMainLayout() {
	// Create header with app title and status
	header := app.createHeader()

	// Create main content area
	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(app.tabs, 1, 0, false).
		AddItem(app.pages, 0, 1, true)

	// Create footer with shortcuts
	footer := app.createFooter()

	// Main layout
	main := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 3, 0, false).
		AddItem(content, 0, 1, true).
		AddItem(footer, 3, 0, false)

	app.app.SetRoot(main, true)
}

// createHeader creates the application header
func (app *App) createHeader() *tview.TextView {
	header := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	headerText := fmt.Sprintf(`[green:black:b]%s[-:-:-] [white:black]v%s[-:-:-]
[yellow:black]%s[-:-:-]`,
		app.config.App.Name,
		app.config.App.Version,
		app.config.App.Description)

	header.SetText(headerText).SetBorder(true)
	return header
}

// createFooter creates the application footer with shortcuts
func (app *App) createFooter() *tview.TextView {
	footer := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	footerText := `[yellow:black]Tab[-:-:-]: Switch tabs | [yellow:black]Ctrl+R[-:-:-]: Refresh | [yellow:black]Ctrl+C[-:-:-]: Quit | [yellow:black]?[-:-:-]: Help`

	footer.SetText(footerText).SetBorder(true)
	return footer
}

// setupKeyBindings sets up global key bindings
func (app *App) setupKeyBindings() {
	app.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			app.nextTab()
			return nil
		case tcell.KeyBacktab:
			app.prevTab()
			return nil
		case tcell.KeyCtrlR:
			app.refresh()
			return nil
		case tcell.KeyCtrlC:
			app.Quit()
			return nil
		case tcell.KeyF1:
			app.showHelp()
			return nil
		}

		// Handle number keys for direct tab switching
		if event.Rune() >= '1' && event.Rune() <= '4' {
			tabIndex := int(event.Rune() - '1')
			if tabIndex < len(app.tabNames) {
				app.switchTab(tabIndex)
			}
			return nil
		}

		return event
	})
}

// switchTab switches to the specified tab
func (app *App) switchTab(index int) {
	if index < 0 || index >= len(app.tabNames) {
		return
	}

	app.mu.Lock()
	app.currentTab = index
	app.mu.Unlock()

	// Use SwitchToPage for cleaner tab switching
	switch index {
	case 0: // Profiles
		app.pages.SwitchToPage("profile")
		app.app.SetFocus(app.profileTab.GetView())
	case 1: // Resources
		app.pages.SwitchToPage("resources")
		app.app.SetFocus(app.resourcesTab.GetView())
	case 2: // Logs
		app.pages.SwitchToPage("logs")
		app.app.SetFocus(app.logsTab.GetView())
	case 3: // Settings
		app.pages.SwitchToPage("settings")
		app.app.SetFocus(app.settingsTab.GetView())
	}

	app.updateTabDisplay()

	logger.Debug("Switched to tab", zap.String("tab", app.tabNames[index]))
}

// nextTab switches to the next tab
func (app *App) nextTab() {
	app.mu.RLock()
	current := app.currentTab
	app.mu.RUnlock()

	next := (current + 1) % len(app.tabNames)
	app.switchTab(next)
}

// prevTab switches to the previous tab
func (app *App) prevTab() {
	app.mu.RLock()
	current := app.currentTab
	app.mu.RUnlock()

	prev := (current - 1 + len(app.tabNames)) % len(app.tabNames)
	app.switchTab(prev)
}

// refresh refreshes the current tab
func (app *App) refresh() {
	app.eventChan <- Event{Type: EventRefresh, Data: nil}
}

// showHelp shows the help dialog
func (app *App) showHelp() {
	helpText := `Swiss Army TUI - Help

Global Shortcuts:
  Tab / Shift+Tab  - Switch between tabs
  1, 2, 3, 4       - Jump to specific tab
  Ctrl+R          - Refresh current tab
  Ctrl+C          - Quit application
  F1 / ?          - Show this help

Profile Tab:
  Enter           - Select AWS profile
  r               - Refresh profiles
  Space           - Test connection

Resources Tab:
  Enter           - View resource details
  r               - Refresh resources
  f               - Filter resources

Press any key to close this help.`

	modal := tview.NewModal().
		SetText(helpText).
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			app.pages.RemovePage("help")
		})

	app.pages.AddPage("help", modal, false, true)
}

// eventHandler handles application events
func (app *App) eventHandler() {
	for {
		select {
		case event := <-app.eventChan:
			app.handleEvent(event)
		case <-app.stopChan:
			return
		case <-app.ctx.Done():
			return
		}
	}
}

// handleEvent handles individual events
func (app *App) handleEvent(event Event) {
	switch event.Type {
	case EventProfileChanged:
		if profileData, ok := event.Data.(map[string]string); ok {
			app.handleProfileChange(profileData)
		}
	case EventRegionChanged:
		if region, ok := event.Data.(string); ok {
			app.handleRegionChange(region)
		}
	case EventRefresh:
		app.handleRefresh()
	case EventError:
		if err, ok := event.Data.(error); ok {
			app.showError(err)
		}
	case EventShowLambdaLogs:
		if data, ok := event.Data.(map[string]string); ok {
			function := data["function"]
			logGroup := data["logGroup"]
			app.switchTab(2)
			if app.logsTab != nil {
				app.logsTab.ShowLambdaLogGroup(function, logGroup)
			}
		}
	}
}

// handleProfileChange handles AWS profile changes
func (app *App) handleProfileChange(data map[string]string) {
	profile := data["profile"]
	region := data["region"]

	logger.Info("Handling profile change",
		zap.String("profile", profile),
		zap.String("region", region))

	// Close existing client
	if app.awsClient != nil {
		app.awsClient.Close()
	}

	// Create new client with selected profile
	client, err := aws.NewClient(profile, region)
	if err != nil {
		app.showError(fmt.Errorf("failed to create AWS client: %w", err))
		return
	}

	app.awsClient = client

	// Update resources tab with new client
	app.resourcesTab.SetAWSClient(client)

	// Show success message
	app.showMessage(fmt.Sprintf("Switched to profile: %s (%s)", profile, region))
}

// handleRegionChange handles AWS region changes
func (app *App) handleRegionChange(region string) {
	if app.awsClient == nil {
		return
	}

	profile := app.awsClient.GetProfile()
	err := app.awsClient.SwitchProfile(profile, region)
	if err != nil {
		app.showError(fmt.Errorf("failed to change region: %w", err))
		return
	}

	app.showMessage(fmt.Sprintf("Changed region to: %s", region))
}

// handleRefresh handles refresh events
func (app *App) handleRefresh() {
	app.mu.RLock()
	currentTab := app.currentTab
	app.mu.RUnlock()

	switch currentTab {
	case 0: // Profiles
		app.profileTab.Refresh()
	case 1: // Resources
		app.resourcesTab.Refresh()
	case 2: // Logs
		app.logsTab.Refresh()
	case 3: // Settings
		app.settingsTab.Refresh()
	}
}

// showError shows an error modal
func (app *App) showError(err error) {
	logger.Error("Application error", zap.Error(err))

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Error: %s", err.Error())).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			app.pages.RemovePage("error")
		})

	app.pages.AddPage("error", modal, false, true)
}

// showMessage shows an info modal
func (app *App) showMessage(message string) {
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			app.pages.RemovePage("message")
		})

	app.pages.AddPage("message", modal, false, true)

	// Auto-close after 2 seconds
	go func() {
		time.Sleep(2 * time.Second)
		app.app.QueueUpdateDraw(func() {
			app.pages.RemovePage("message")
		})
	}()
}

// Run starts the TUI application
func (app *App) Run() error {
	logger.Info("Starting TUI application")

	// Enable mouse and configure screen settings to prevent duplication
	app.app.EnableMouse(app.config.UI.MouseEnabled)

	if err := app.app.Run(); err != nil {
		return fmt.Errorf("failed to run TUI application: %w", err)
	}

	return nil
}

// Quit gracefully shuts down the application
func (app *App) Quit() {
	logger.Info("Shutting down TUI application")

	// Close AWS client
	if app.awsClient != nil {
		app.awsClient.Close()
	}

	// Stop event handler
	close(app.stopChan)

	// Cancel context
	app.cancel()

	// Stop the application
	app.app.Stop()
}

// GetAWSClient returns the current AWS client
func (app *App) GetAWSClient() *aws.Client {
	return app.awsClient
}

// GetProfileManager returns the profile manager
func (app *App) GetProfileManager() *aws.ProfileManager {
	return app.profileManager
}
