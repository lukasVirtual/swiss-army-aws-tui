package ui

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"swiss-army-tui/pkg/logger"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"go.uber.org/zap"
)

// LogsTab represents the logs viewing tab
type LogsTab struct {
	// Core components
	view *tview.Flex

	// UI components
	logSourceList *tview.List
	logView       *tview.TextView
	filterInput   *tview.InputField
	statusText    *tview.TextView

	// State
	selectedSource string
	logs           map[string][]LogEntry
	filteredLogs   []LogEntry
	mu             sync.RWMutex
	autoScroll     bool
	maxLines       int
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
	Source    string
	Fields    map[string]interface{}
}

// LogSource represents a log source
type LogSource struct {
	Name        string
	DisplayName string
	Type        string
	Path        string
	Enabled     bool
}

var logSources = []LogSource{
	{Name: "app", DisplayName: "Application Logs", Type: "memory", Path: "", Enabled: true},
	{Name: "aws-sdk", DisplayName: "AWS SDK Logs", Type: "memory", Path: "", Enabled: false},
	{Name: "system", DisplayName: "System Logs", Type: "file", Path: "/var/log/system.log", Enabled: false},
	{Name: "cloudwatch", DisplayName: "CloudWatch Logs", Type: "aws", Path: "", Enabled: false},
	{Name: "docker", DisplayName: "Docker Logs", Type: "command", Path: "docker logs", Enabled: false},
	{Name: "kubectl", DisplayName: "Kubernetes Logs", Type: "command", Path: "kubectl logs", Enabled: false},
}

// NewLogsTab creates a new logs tab
func NewLogsTab() (*LogsTab, error) {
	tab := &LogsTab{
		logs:       make(map[string][]LogEntry),
		autoScroll: true,
		maxLines:   1000,
	}

	if err := tab.initializeUI(); err != nil {
		return nil, fmt.Errorf("failed to initialize logs tab UI: %w", err)
	}

	// Initialize with some sample application logs
	tab.initializeAppLogs()

	return tab, nil
}

// initializeUI initializes the UI components
func (lt *LogsTab) initializeUI() error {
	// Create log source list
	lt.logSourceList = tview.NewList().
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorBlack).
		SetSelectedBackgroundColor(tcell.ColorWhite).
		ShowSecondaryText(true)

	lt.logSourceList.SetBorder(true).SetTitle(" Log Sources ").SetTitleAlign(tview.AlignLeft)

	// Set up log source handlers
	lt.logSourceList.SetSelectedFunc(lt.onSourceSelected)
	lt.logSourceList.SetChangedFunc(lt.onSourceHighlighted)

	// Add key bindings for source list
	lt.logSourceList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'r':
			lt.Refresh()
			return nil
		case 'c':
			lt.clearLogs()
			return nil
		case 's':
			lt.toggleAutoScroll()
			return nil
		case 'f':
			lt.focusFilter()
			return nil
		}
		return event
	})

	// Create filter input
	lt.filterInput = tview.NewInputField().
		SetLabel("Filter: ").
		SetFieldWidth(0).
		SetChangedFunc(lt.onFilterChanged)

	lt.filterInput.SetBorder(true).SetTitle(" Filter Logs ").SetTitleAlign(tview.AlignLeft)

	// Create log view
	lt.logView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(true)

	lt.logView.SetBorder(true).SetTitle(" Logs ").SetTitleAlign(tview.AlignLeft)

	// Add key bindings for log view
	lt.logView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'r':
			lt.Refresh()
			return nil
		case 'c':
			lt.clearLogs()
			return nil
		case 's':
			lt.toggleAutoScroll()
			return nil
		case 'f':
			lt.focusFilter()
			return nil
		case 'g':
			lt.logView.ScrollToBeginning()
			return nil
		case 'G':
			lt.logView.ScrollToEnd()
			return nil
		}
		return event
	})

	// Create status text
	lt.statusText = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	lt.statusText.SetBorder(true).SetTitle(" Status ").SetTitleAlign(tview.AlignLeft)
	lt.updateStatus("Ready", "green")

	// Load sources into list
	lt.loadLogSources()

	// Create layout
	leftPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(lt.logSourceList, 0, 2, true).
		AddItem(lt.filterInput, 3, 0, false).
		AddItem(lt.statusText, 5, 0, false)

	rightPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(lt.logView, 0, 1, false)

	lt.view = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(leftPanel, 25, 0, true).
		AddItem(rightPanel, 0, 1, false)

	return nil
}

// loadLogSources loads log sources into the list
func (lt *LogsTab) loadLogSources() {
	lt.logSourceList.Clear()

	for i, source := range logSources {
		mainText := source.DisplayName
		secondaryText := source.Type

		if !source.Enabled {
			mainText = fmt.Sprintf("[gray]%s (Disabled)[-]", mainText)
			secondaryText = "Not available"
		}

		lt.logSourceList.AddItem(mainText, secondaryText, rune('0'+i%10), func() {
			if source.Enabled {
				lt.selectSource(source.Name)
			}
		})
	}

	// Select first available source
	for _, source := range logSources {
		if source.Enabled {
			lt.selectSource(source.Name)
			break
		}
	}
}

// onSourceSelected handles log source selection
func (lt *LogsTab) onSourceSelected(index int, mainText, secondaryText string, shortcut rune) {
	if index >= 0 && index < len(logSources) {
		source := logSources[index]
		if source.Enabled {
			lt.selectSource(source.Name)
		}
	}
}

// onSourceHighlighted handles source highlighting
func (lt *LogsTab) onSourceHighlighted(index int, mainText, secondaryText string, shortcut rune) {
	// Update status or info based on highlighted source
	if index >= 0 && index < len(logSources) {
		source := logSources[index]
		lt.updateStatus(fmt.Sprintf("Source: %s (%s)", source.DisplayName, source.Type), "blue")
	}
}

// selectSource selects a log source and displays its logs
func (lt *LogsTab) selectSource(sourceName string) {
	lt.mu.Lock()
	lt.selectedSource = sourceName
	lt.mu.Unlock()

	logger.Debug("Selecting log source", zap.String("source", sourceName))

	// Load logs for the selected source
	lt.loadLogsForSource(sourceName)
}

// loadLogsForSource loads logs for a specific source
func (lt *LogsTab) loadLogsForSource(sourceName string) {
	lt.mu.RLock()
	logs, exists := lt.logs[sourceName]
	lt.mu.RUnlock()

	if !exists {
		// Initialize empty logs for this source
		lt.mu.Lock()
		lt.logs[sourceName] = []LogEntry{}
		lt.mu.Unlock()
		logs = []LogEntry{}
	}

	lt.updateLogDisplay(logs)
	lt.updateStatus(fmt.Sprintf("Showing %d log entries from %s", len(logs), sourceName), "green")
}

// updateLogDisplay updates the log display with the given logs
func (lt *LogsTab) updateLogDisplay(logs []LogEntry) {
	lt.filteredLogs = logs
	lt.applyFilter()
}

// applyFilter applies the current filter to logs
func (lt *LogsTab) applyFilter() {
	if lt.filterInput == nil || lt.logView == nil {
		return
	}
	lt.logView.Clear() // Clear existing logs to prevent duplication

	filterText := strings.ToLower(strings.TrimSpace(lt.filterInput.GetText()))

	var filtered []LogEntry
	if filterText == "" {
		filtered = lt.filteredLogs
	} else {
		for _, log := range lt.filteredLogs {
			if strings.Contains(strings.ToLower(log.Message), filterText) ||
				strings.Contains(strings.ToLower(log.Level), filterText) ||
				strings.Contains(strings.ToLower(log.Source), filterText) {
				filtered = append(filtered, log)
			}
		}
	}

	// Sort logs by timestamp
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.Before(filtered[j].Timestamp)
	})

	// Convert to display text
	var logText strings.Builder

	for _, log := range filtered {
		// Color-code log levels
		levelColor := "white"
		switch strings.ToUpper(log.Level) {
		case "ERROR", "FATAL":
			levelColor = "red"
		case "WARN", "WARNING":
			levelColor = "yellow"
		case "INFO":
			levelColor = "green"
		case "DEBUG":
			levelColor = "blue"
		}

		timestamp := log.Timestamp.Format("15:04:05.000")
		logText.WriteString(fmt.Sprintf("[gray]%s[-] [%s]%-5s[-] %s\n",
			timestamp, levelColor, strings.ToUpper(log.Level), log.Message))

		// Add fields if any
		if len(log.Fields) > 0 {
			var fieldKeys []string
			for key := range log.Fields {
				fieldKeys = append(fieldKeys, key)
			}
			sort.Strings(fieldKeys)

			for _, key := range fieldKeys {
				logText.WriteString(fmt.Sprintf("  [blue]%s:[-] %v\n", key, log.Fields[key]))
			}
		}
	}

	lt.logView.SetText(logText.String())

	// Auto-scroll to bottom if enabled
	if lt.autoScroll {
		lt.logView.ScrollToEnd()
	}

	// Update title with count
	title := fmt.Sprintf(" Logs (%d", len(filtered))
	if len(filtered) != len(lt.filteredLogs) {
		title += fmt.Sprintf(" of %d", len(lt.filteredLogs))
	}
	title += ") "
	lt.logView.SetTitle(title)
}

// onFilterChanged handles filter text changes
func (lt *LogsTab) onFilterChanged(text string) {
	lt.applyFilter()
}

// addLogEntry adds a new log entry to a specific source
func (lt *LogsTab) addLogEntry(sourceName string, entry LogEntry) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if lt.logs[sourceName] == nil {
		lt.logs[sourceName] = []LogEntry{}
	}

	lt.logs[sourceName] = append(lt.logs[sourceName], entry)

	// Limit the number of log entries to prevent memory issues
	if len(lt.logs[sourceName]) > lt.maxLines {
		lt.logs[sourceName] = lt.logs[sourceName][len(lt.logs[sourceName])-lt.maxLines:]
	}

	// Update display if this is the currently selected source
	if lt.selectedSource == sourceName {
		lt.updateLogDisplay(lt.logs[sourceName])
	}
}

// initializeAppLogs initializes sample application logs
func (lt *LogsTab) initializeAppLogs() {
	sampleLogs := []LogEntry{
		{
			Timestamp: time.Now().Add(-5 * time.Minute),
			Level:     "INFO",
			Message:   "Application started successfully",
			Source:    "app",
			Fields:    map[string]interface{}{"version": "1.0.0", "pid": 12345},
		},
		{
			Timestamp: time.Now().Add(-4 * time.Minute),
			Level:     "INFO",
			Message:   "AWS profile manager initialized",
			Source:    "app",
			Fields:    map[string]interface{}{"profiles_found": 3},
		},
		{
			Timestamp: time.Now().Add(-3 * time.Minute),
			Level:     "DEBUG",
			Message:   "Loading configuration from file",
			Source:    "app",
			Fields:    map[string]interface{}{"config_path": "~/.swiss-army-tui/config.yaml"},
		},
		{
			Timestamp: time.Now().Add(-2 * time.Minute),
			Level:     "INFO",
			Message:   "TUI application initialized successfully",
			Source:    "app",
			Fields:    map[string]interface{}{"tabs_created": 4},
		},
		{
			Timestamp: time.Now().Add(-1 * time.Minute),
			Level:     "WARN",
			Message:   "Failed to load some AWS profiles",
			Source:    "app",
			Fields:    map[string]interface{}{"error": "credentials file not found"},
		},
		{
			Timestamp: time.Now(),
			Level:     "INFO",
			Message:   "Logs tab initialized",
			Source:    "app",
			Fields:    map[string]interface{}{"sources_available": len(logSources)},
		},
	}

	lt.mu.Lock()
	lt.logs["app"] = sampleLogs
	lt.mu.Unlock()
}

// clearLogs clears logs for the current source
func (lt *LogsTab) clearLogs() {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if lt.selectedSource != "" {
		lt.logs[lt.selectedSource] = []LogEntry{}
		lt.updateLogDisplay([]LogEntry{})
		lt.updateStatus("Logs cleared", "yellow")
	}
}

// toggleAutoScroll toggles auto-scroll functionality
func (lt *LogsTab) toggleAutoScroll() {
	lt.autoScroll = !lt.autoScroll
	status := "disabled"
	if lt.autoScroll {
		status = "enabled"
		lt.logView.ScrollToEnd()
	}
	lt.updateStatus(fmt.Sprintf("Auto-scroll %s", status), "blue")
}

// focusFilter focuses on the filter input field
func (lt *LogsTab) focusFilter() {
	// This would be called from the application level
}

// updateStatus updates the status display
func (lt *LogsTab) updateStatus(message, color string) {
	if lt.statusText == nil {
		return
	}
	lt.statusText.Clear() // Clear existing status to prevent duplication

	timestamp := time.Now().Format("15:04:05")

	autoScrollStatus := "ON"
	if !lt.autoScroll {
		autoScrollStatus = "OFF"
	}

	statusText := fmt.Sprintf("[%s]%s[-]\n[gray]%s[-]\n[blue]Auto-scroll: %s[-]",
		color, message, timestamp, autoScrollStatus)
	lt.statusText.SetText(statusText)
}

// Refresh refreshes the current log source
func (lt *LogsTab) Refresh() {
	lt.mu.RLock()
	source := lt.selectedSource
	lt.mu.RUnlock()

	if source == "" {
		lt.updateStatus("No source selected", "yellow")
		return
	}

	logger.Debug("Refreshing logs", zap.String("source", source))

	switch source {
	case "app":
		// Add a new refresh log entry
		lt.addLogEntry("app", LogEntry{
			Timestamp: time.Now(),
			Level:     "INFO",
			Message:   "Logs refreshed manually",
			Source:    "app",
			Fields:    map[string]interface{}{"action": "refresh"},
		})
	default:
		// For other sources, just reload existing logs
		lt.loadLogsForSource(source)
	}

	lt.updateStatus("Logs refreshed", "green")
}

// AddApplicationLog adds a log entry to the application logs
func (lt *LogsTab) AddApplicationLog(level, message string, fields map[string]interface{}) {
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Source:    "app",
		Fields:    fields,
	}
	lt.addLogEntry("app", entry)
}

// GetView returns the main view component
func (lt *LogsTab) GetView() tview.Primitive {
	return lt.view
}

// GetLogCount returns the number of logs for a specific source
func (lt *LogsTab) GetLogCount(source string) int {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	if logs, exists := lt.logs[source]; exists {
		return len(logs)
	}
	return 0
}

// ExportLogs exports logs to a file (placeholder)
func (lt *LogsTab) ExportLogs(filename string) error {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create export file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	// Export all logs from current source
	if logs, exists := lt.logs[lt.selectedSource]; exists {
		for _, log := range logs {
			line := fmt.Sprintf("%s [%s] %s\n",
				log.Timestamp.Format("2006-01-02 15:04:05.000"),
				strings.ToUpper(log.Level),
				log.Message)
			writer.WriteString(line)
		}
	}

	return nil
}
