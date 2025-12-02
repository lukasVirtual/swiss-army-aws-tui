package ui

import (
	"context"
	"fmt"
	"sort"
	"time"

	"swiss-army-tui/internal/aws"
	"swiss-army-tui/pkg/logger"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"go.uber.org/zap"
)

// ProfileTab represents the AWS profile selection tab
type ProfileTab struct {
	// Core components
	view           *tview.Flex
	profileManager *aws.ProfileManager
	eventChan      chan<- Event

	// UI components
	profileList  *tview.List
	profileInfo  *tview.TextView
	statusText   *tview.TextView
	regionSelect *tview.DropDown

	// State
	selectedProfile *aws.Profile
	selectedRegion  string
	profiles        map[string]*aws.Profile
}

// NewProfileTab creates a new profile tab
func NewProfileTab(profileManager *aws.ProfileManager, eventChan chan<- Event) (*ProfileTab, error) {
	tab := &ProfileTab{
		profileManager: profileManager,
		eventChan:      eventChan,
		profiles:       make(map[string]*aws.Profile),
		selectedRegion: "us-east-1",
	}

	if err := tab.initializeUI(); err != nil {
		return nil, fmt.Errorf("failed to initialize profile tab UI: %w", err)
	}

	// Load profiles
	tab.loadProfiles()

	return tab, nil
}

// initializeUI initializes the UI components
func (pt *ProfileTab) initializeUI() error {
	// Create profile list
	pt.profileList = tview.NewList().
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorBlack).
		SetSelectedBackgroundColor(tcell.ColorWhite).
		ShowSecondaryText(true)

	pt.profileList.SetBorder(true).SetTitle(" AWS Profiles ").SetTitleAlign(tview.AlignLeft)

	// Set up profile list selection handler
	pt.profileList.SetSelectedFunc(pt.onProfileSelected)
	pt.profileList.SetChangedFunc(pt.onProfileHighlighted)

	// Add key bindings for profile list
	pt.profileList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'r':
			pt.Refresh()
			return nil
		case ' ':
			pt.testConnection()
			return nil
		}
		return event
	})

	// Create profile info panel
	pt.profileInfo = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true)

	pt.profileInfo.SetBorder(true).SetTitle(" Profile Details ").SetTitleAlign(tview.AlignLeft)

	// Create region selector
	pt.regionSelect = tview.NewDropDown().
		SetLabel("Region: ").
		SetOptions(getAWSRegions(), pt.onRegionSelected)

	pt.regionSelect.SetBorder(true).SetTitle(" AWS Region ").SetTitleAlign(tview.AlignLeft)

	// Set default region
	pt.regionSelect.SetCurrentOption(findRegionIndex("eu-central-1"))

	// Create status text
	pt.statusText = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	pt.statusText.SetBorder(true).SetTitle(" Status ").SetTitleAlign(tview.AlignLeft)
	pt.updateStatus("Ready", "white")

	// Create layout
	leftPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(pt.profileList, 0, 2, true).
		AddItem(pt.regionSelect, 7, 0, false).
		AddItem(pt.statusText, 5, 0, false)

	pt.view = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(leftPanel, 0, 1, true).
		AddItem(pt.profileInfo, 0, 1, false)

	return nil
}

// loadProfiles loads AWS profiles into the list
func (pt *ProfileTab) loadProfiles() {
	logger.Debug("Loading AWS profiles in UI")

	// Reload profiles from profile manager
	if err := pt.profileManager.LoadProfiles(); err != nil {
		logger.Error("Failed to load profiles", zap.Error(err))
		pt.updateStatus("Error loading profiles", "red")
		return
	}

	pt.profiles = pt.profileManager.GetProfiles()

	// Clear existing list
	pt.profileList.Clear()
	pt.profileInfo.SetText("") // Clear profile info
	pt.statusText.SetText("")  // Clear status text

	if len(pt.profiles) == 0 {
		pt.profileList.AddItem("No profiles found", "Check ~/.aws/config and ~/.aws/credentials", 0, nil)
		pt.updateStatus("No profiles found", "yellow")
		return
	}

	// Sort profiles by name
	profileNames := make([]string, 0, len(pt.profiles))
	for name := range pt.profiles {
		profileNames = append(profileNames, name)
	}
	sort.Strings(profileNames)

	// Add profiles to list
	for i, name := range profileNames {
		profile := pt.profiles[name]
		mainText := name
		if name == "default" {
			mainText = fmt.Sprintf("[yellow]%s[-] (default)", name)
		}

		secondaryText := fmt.Sprintf("Region: %s | Source: %s",
			getProfileRegion(profile), profile.Source)

		pt.profileList.AddItem(mainText, secondaryText, rune('0'+i%10), func() {
			pt.selectProfile(name)
		})
	}

	pt.updateStatus(fmt.Sprintf("Found %d profiles", len(pt.profiles)), "green")
	logger.Info("Loaded AWS profiles", zap.Int("count", len(pt.profiles)))
}

// onProfileSelected handles profile selection
func (pt *ProfileTab) onProfileSelected(index int, mainText, secondaryText string, shortcut rune) {
	profileNames := pt.getSortedProfileNames()
	if index >= 0 && index < len(profileNames) {
		profileName := profileNames[index]
		pt.selectProfile(profileName)
	}
}

// onProfileHighlighted handles profile highlighting (when cursor moves)
func (pt *ProfileTab) onProfileHighlighted(index int, mainText, secondaryText string, shortcut rune) {
	profileNames := pt.getSortedProfileNames()
	if index >= 0 && index < len(profileNames) {
		profileName := profileNames[index]
		if profile, exists := pt.profiles[profileName]; exists {
			pt.updateProfileInfo(profile)
		}
	}
}

// selectProfile selects and activates a profile
func (pt *ProfileTab) selectProfile(profileName string) {
	logger.Info("Selecting AWS profile", zap.String("profile", profileName))

	profile, exists := pt.profiles[profileName]
	if !exists {
		pt.updateStatus("Profile not found", "red")
		return
	}

	pt.selectedProfile = profile
	pt.updateProfileInfo(profile)

	// Get selected region
	currentRegion := pt.selectedRegion
	if profile.Region != "" {
		currentRegion = profile.Region
		// Update region dropdown
		if regionIndex := findRegionIndex(profile.Region); regionIndex >= 0 {
			pt.regionSelect.SetCurrentOption(regionIndex)
		}
	}

	// Notify about profile change
	pt.eventChan <- Event{
		Type: EventProfileChanged,
		Data: map[string]string{
			"profile": profileName,
			"region":  currentRegion,
		},
	}

	pt.profileList.Clear() // Clear existing list to prevent duplication
	pt.updateStatus(fmt.Sprintf("Selected profile: %s", profileName), "green")
}

// onRegionSelected handles region selection
func (pt *ProfileTab) onRegionSelected(option string, index int) {
	pt.selectedRegion = option

	if pt.selectedProfile != nil {
		logger.Info("Region changed",
			zap.String("profile", pt.selectedProfile.Name),
			zap.String("region", option))

		pt.eventChan <- Event{
			Type: EventRegionChanged,
			Data: option,
		}

		pt.updateStatus(fmt.Sprintf("Changed region to: %s", option), "green")
	}
}

// updateProfileInfo updates the profile information panel
func (pt *ProfileTab) updateProfileInfo(profile *aws.Profile) {
	// Guard against nil profileInfo during initialization
	if pt.profileInfo == nil {
		return
	}

	if profile == nil {
		pt.profileInfo.SetText("No profile selected")
		return
	}

	info := fmt.Sprintf(`[yellow]Profile Name:[-] %s

[yellow]Region:[-] %s
[yellow]Output Format:[-] %s
[yellow]Source:[-] %s

`, profile.Name,
		getProfileRegion(profile),
		getProfileOutput(profile),
		profile.Source)

	if profile.RoleARN != "" {
		info += fmt.Sprintf("[yellow]Role ARN:[-] %s\n", profile.RoleARN)
	}

	if profile.SourceProfile != "" {
		info += fmt.Sprintf("[yellow]Source Profile:[-] %s\n", profile.SourceProfile)
	}

	info += `
[blue]Actions:[-]
• [white]Enter[-]: Select profile
• [white]Space[-]: Test connection
• [white]r[-]: Refresh profiles

[blue]Tips:[-]
• Profiles are loaded from ~/.aws/config and ~/.aws/credentials
• Use the region dropdown to override the profile region
• Selected profile will be used for all AWS operations`

	pt.profileInfo.SetText(info)
}

// testConnection tests the connection with the selected profile
func (pt *ProfileTab) testConnection() {
	if pt.selectedProfile == nil {
		pt.updateStatus("No profile selected", "yellow")
		return
	}

	pt.updateStatus("Testing connection...", "yellow")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client, err := aws.NewClient(pt.selectedProfile.Name, pt.selectedRegion)
		if err != nil {
			pt.updateStatus("Connection failed", "red")
			logger.Error("Failed to create AWS client for connection test", zap.Error(err))
			return
		}
		defer client.Close()

		if err := client.TestConnection(ctx); err != nil {
			pt.updateStatus("Connection failed", "red")
			logger.Error("Connection test failed", zap.Error(err))
		} else {
			accountID := client.GetAccountID()
			pt.updateStatus(fmt.Sprintf("Connected to account: %s", accountID), "green")
			logger.Info("Connection test successful", zap.String("account_id", accountID))
		}
	}()
}

// updateStatus updates the status display
func (pt *ProfileTab) updateStatus(message, color string) {
	// Guard against nil statusText during initialization
	if pt.statusText == nil {
		return
	}

	timestamp := time.Now().Format("15:04:05")
	statusText := fmt.Sprintf("[%s]%s[-]\n[gray]%s[-]", color, message, timestamp)
	pt.statusText.SetText(statusText)
}

// Refresh refreshes the profile list
func (pt *ProfileTab) Refresh() {
	logger.Debug("Refreshing profile tab")
	pt.profileList.Clear() // Clear existing list to prevent duplication
	pt.updateStatus("Refreshing...", "yellow")
	pt.loadProfiles()
}

// getSortedProfileNames returns a sorted list of profile names
func (pt *ProfileTab) getSortedProfileNames() []string {
	names := make([]string, 0, len(pt.profiles))
	for name := range pt.profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetView returns the main view component
func (pt *ProfileTab) GetView() tview.Primitive {
	return pt.view
}

// getAWSRegions returns a list of AWS regions
func getAWSRegions() []string {
	return []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2",
		"eu-west-1", "eu-west-2", "eu-west-3", "eu-central-1", "eu-north-1",
		"ap-southeast-1", "ap-southeast-2", "ap-northeast-1", "ap-northeast-2", "ap-south-1",
		"ca-central-1", "sa-east-1", "af-south-1", "me-south-1",
		"ap-east-1", "ap-northeast-3", "eu-south-1",
	}
}

// findRegionIndex finds the index of a region in the regions list
func findRegionIndex(region string) int {
	regions := getAWSRegions()
	for i, r := range regions {
		if r == region {
			return i
		}
	}
	return 0 // Default to us-east-1
}

// getProfileRegion returns the profile's region or default
func getProfileRegion(profile *aws.Profile) string {
	if profile.Region != "" {
		return profile.Region
	}
	return "us-east-1 (default)"
}

// getProfileOutput returns the profile's output format or default
func getProfileOutput(profile *aws.Profile) string {
	if profile.Output != "" {
		return profile.Output
	}
	return "json (default)"
}
