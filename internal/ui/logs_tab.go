package ui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"swiss-army-tui/internal/aws"
	"swiss-army-tui/internal/aws/clients"
	"swiss-army-tui/pkg/logger"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"go.uber.org/zap"
)

type LogsTab struct {
	view *tview.Flex

	logSourceList *tview.List
	logView       *tview.TextView
	filterInput   *tview.InputField
	statusText    *tview.TextView

	selectedSource string
	logs           map[string][]LogEntry
	filteredLogs   []LogEntry
	mu             sync.RWMutex
	autoScroll     bool
	maxLines       int
	activeLogGroup string
	awsClient      *aws.Client

	// CloudWatch Logs specific fields
	cloudWatchCtx    context.Context
	cloudWatchCancel context.CancelFunc
	tailingActive    bool
}

type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
	Source    string
	Fields    map[string]interface{}
}

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
	{Name: "cloudwatch", DisplayName: "CloudWatch Logs", Type: "aws", Path: "", Enabled: true},
	{Name: "docker", DisplayName: "Docker Logs", Type: "command", Path: "docker logs", Enabled: false},
	{Name: "kubectl", DisplayName: "Kubernetes Logs", Type: "command", Path: "kubectl logs", Enabled: false},
}

func NewLogsTab() (*LogsTab, error) {
	tab := &LogsTab{
		logs:       make(map[string][]LogEntry),
		autoScroll: true,
		maxLines:   1000,
	}

	if err := tab.initializeUI(); err != nil {
		return nil, fmt.Errorf("failed to initialize logs tab UI: %w", err)
	}

	tab.initializeAppLogs()

	return tab, nil
}

func (lt *LogsTab) initializeUI() error {

	lt.logSourceList = tview.NewList().
		SetMainTextColor(tcell.ColorWhite).
		SetSelectedTextColor(tcell.ColorBlack).
		SetSelectedBackgroundColor(tcell.ColorWhite).
		ShowSecondaryText(true)

	lt.logSourceList.SetBorder(true).SetTitle(" Log Sources ").SetTitleAlign(tview.AlignLeft)

	lt.logSourceList.SetSelectedFunc(lt.onSourceSelected)
	lt.logSourceList.SetChangedFunc(lt.onSourceHighlighted)

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

	lt.filterInput = tview.NewInputField().
		SetLabel("Filter: ").
		SetFieldWidth(0).
		SetChangedFunc(lt.onFilterChanged)

	lt.filterInput.SetBorder(true).SetTitle(" Filter Logs ").SetTitleAlign(tview.AlignLeft)

	lt.logView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(true)

	lt.logView.SetBorder(true).SetTitle(" Logs ").SetTitleAlign(tview.AlignLeft)

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

	lt.statusText = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	lt.statusText.SetBorder(true).SetTitle(" Status ").SetTitleAlign(tview.AlignLeft)
	lt.updateStatus("Ready", "green")

	lt.loadLogSources()

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

	for _, source := range logSources {
		if source.Enabled {
			lt.selectSource(source.Name)
			break
		}
	}
}

func (lt *LogsTab) onSourceSelected(index int, mainText, secondaryText string, shortcut rune) {
	if index >= 0 && index < len(logSources) {
		source := logSources[index]
		if source.Enabled {
			lt.selectSource(source.Name)
		}
	}
}

func (lt *LogsTab) onSourceHighlighted(index int, mainText, secondaryText string, shortcut rune) {

	if index >= 0 && index < len(logSources) {
		source := logSources[index]
		lt.updateStatus(fmt.Sprintf("Source: %s (%s)", source.DisplayName, source.Type), "blue")
	}
}

func (lt *LogsTab) selectSource(sourceName string) {
	lt.mu.Lock()
	lt.selectedSource = sourceName
	lt.mu.Unlock()

	logger.Debug("Selecting log source", zap.String("source", sourceName))

	lt.loadLogsForSource(sourceName)
}

func (lt *LogsTab) loadLogsForSource(sourceName string) {
	lt.mu.RLock()
	logs, exists := lt.logs[sourceName]
	lt.mu.RUnlock()

	if !exists {
		lt.mu.Lock()
		switch sourceName {
		case "cloudwatch":
			logger.Info("CloudWatch logs activated...")
			lt.logs[sourceName] = []LogEntry{}
			if lt.activeLogGroup != "" && lt.awsClient != nil {
				go lt.loadCloudWatchLogs(lt.activeLogGroup)
			} else {
				lt.updateStatus("No active log group or AWS client available", "yellow")
			}
		default:
			lt.logs[sourceName] = []LogEntry{}
		}
		lt.mu.Unlock()
		logs = []LogEntry{}
	}

	lt.updateLogDisplay(logs)
	lt.updateStatus(fmt.Sprintf("Showing %d log entries from %s", len(logs), sourceName), "green")
}

func (lt *LogsTab) updateLogDisplay(logs []LogEntry) {
	lt.filteredLogs = logs
	lt.applyFilter()
}

func (lt *LogsTab) applyFilter() {
	if lt.filterInput == nil || lt.logView == nil {
		return
	}
	lt.logView.Clear()

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

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.Before(filtered[j].Timestamp)
	})

	var logText strings.Builder

	for _, log := range filtered {

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

	if lt.autoScroll {
		lt.logView.ScrollToEnd()
	}

	title := fmt.Sprintf(" Logs (%d", len(filtered))
	if len(filtered) != len(lt.filteredLogs) {
		title += fmt.Sprintf(" of %d", len(lt.filteredLogs))
	}
	title += ") "
	lt.logView.SetTitle(title)
}

func (lt *LogsTab) onFilterChanged(text string) {
	lt.applyFilter()
}

func (lt *LogsTab) addLogEntry(sourceName string, entry LogEntry) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if lt.logs[sourceName] == nil {
		lt.logs[sourceName] = []LogEntry{}
	}

	lt.logs[sourceName] = append(lt.logs[sourceName], entry)

	if len(lt.logs[sourceName]) > lt.maxLines {
		lt.logs[sourceName] = lt.logs[sourceName][len(lt.logs[sourceName])-lt.maxLines:]
	}

	if lt.selectedSource == sourceName {
		lt.updateLogDisplay(lt.logs[sourceName])
	}
}

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

func (lt *LogsTab) clearLogs() {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if lt.selectedSource != "" {
		lt.logs[lt.selectedSource] = []LogEntry{}
		lt.updateLogDisplay([]LogEntry{})
		lt.updateStatus("Logs cleared", "yellow")
	}
}

func (lt *LogsTab) toggleAutoScroll() {
	lt.autoScroll = !lt.autoScroll
	status := "disabled"
	if lt.autoScroll {
		status = "enabled"
		lt.logView.ScrollToEnd()
	}
	lt.updateStatus(fmt.Sprintf("Auto-scroll %s", status), "blue")
}

func (lt *LogsTab) focusFilter() {

}

func (lt *LogsTab) updateStatus(message, color string) {
	if lt.statusText == nil {
		return
	}
	lt.statusText.Clear()

	timestamp := time.Now().Format("15:04:05")

	autoScrollStatus := "ON"
	if !lt.autoScroll {
		autoScrollStatus = "OFF"
	}

	statusText := fmt.Sprintf("[%s]%s[-]\n[gray]%s[-]\n[blue]Auto-scroll: %s[-]",
		color, message, timestamp, autoScrollStatus)
	lt.statusText.SetText(statusText)
}

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
		lt.addLogEntry("app", LogEntry{
			Timestamp: time.Now(),
			Level:     "INFO",
			Message:   "Logs refreshed manually",
			Source:    "app",
			Fields:    map[string]interface{}{"action": "refresh"},
		})
	case "cloudwatch":
		lt.stopTailing()
		if lt.activeLogGroup != "" && lt.awsClient != nil {
			go lt.loadCloudWatchLogs(lt.activeLogGroup)
		} else {
			lt.updateStatus("No active log group or AWS client available", "yellow")
		}
	default:
		lt.loadLogsForSource(source)
	}

	lt.updateStatus("Logs refreshed", "green")
}

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

func (lt *LogsTab) GetView() tview.Primitive {
	return lt.view
}

func (lt *LogsTab) ShowLambdaLogGroup(functionName, logGroup string) {
	if lt == nil {
		return
	}

	lt.mu.Lock()
	lt.activeLogGroup = logGroup
	lt.mu.Unlock()

	index := -1
	for i, source := range logSources {
		if source.Name == "cloudwatch" && source.Enabled {
			index = i
			break
		}
	}

	if index >= 0 {
		lt.logSourceList.SetCurrentItem(index)
		lt.selectSource("cloudwatch")
	}

	message := fmt.Sprintf("Lambda %s - CloudWatch log group: %s", functionName, logGroup)
	lt.updateStatus(message, "blue")
}

// SetAWSClient sets the AWS client for the LogsTab
func (lt *LogsTab) SetAWSClient(client *aws.Client) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	lt.awsClient = client
	if client != nil {
		logger.Info("AWS client set for LogsTab")
	} else {
		logger.Info("AWS client removed from LogsTab")
	}
}

// loadCloudWatchLogs loads logs from CloudWatch Logs
func (lt *LogsTab) loadCloudWatchLogs(logGroupName string) {
	if lt.awsClient == nil {
		lt.updateStatus("No AWS client available", "red")
		return
	}

	cloudWatchService := lt.awsClient.GetCloudWatchLogsService()
	if cloudWatchService == nil {
		lt.updateStatus("CloudWatch Logs service not available", "red")
		return
	}

	lt.updateStatus(fmt.Sprintf("Loading CloudWatch logs from %s...", logGroupName), "yellow")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get log streams
	streams, err := cloudWatchService.DescribeLogStreams(ctx, logGroupName, 10)
	if err != nil {
		logger.Error("Failed to describe log streams", zap.String("logGroup", logGroupName), zap.Error(err))
		lt.updateStatus(fmt.Sprintf("Failed to get log streams: %s", err.Error()), "red")
		return
	}

	if len(streams) == 0 {
		lt.updateStatus(fmt.Sprintf("No log streams found in %s", logGroupName), "yellow")
		return
	}

	// Load events from the most recent streams
	var allEvents []clients.LogEvent
	for _, stream := range streams {
		events, _, err := cloudWatchService.GetLogEvents(ctx, logGroupName, stream.LogStreamName, 50, false)
		if err != nil {
			logger.Error("Failed to get log events", zap.String("logGroup", logGroupName), zap.String("stream", stream.LogStreamName), zap.Error(err))
			continue
		}
		allEvents = append(allEvents, events...)
	}

	// Convert to LogEntry format and add to logs
	lt.mu.Lock()
	var logEntries []LogEntry
	for _, event := range allEvents {
		entry := LogEntry{
			Timestamp: time.Now(), // We'll use the event timestamp below
			Level:     "INFO",
			Message:   event.Message,
			Source:    "cloudwatch",
			Fields:    make(map[string]interface{}),
		}

		if event.Timestamp != 0 {
			entry.Timestamp = time.UnixMilli(event.Timestamp)
		}
		if event.IngestionTime != 0 {
			entry.Fields["ingestionTime"] = time.UnixMilli(event.IngestionTime).Format("2006-01-02 15:04:05")
		}

		logEntries = append(logEntries, entry)
	}

	// Sort by timestamp (newest first)
	sort.Slice(logEntries, func(i, j int) bool {
		return logEntries[i].Timestamp.After(logEntries[j].Timestamp)
	})

	lt.logs["cloudwatch"] = logEntries
	lt.mu.Unlock()

	// Update display if cloudwatch is the selected source
	lt.mu.RLock()
	selectedSource := lt.selectedSource
	lt.mu.RUnlock()

	if selectedSource == "cloudwatch" {
		lt.updateLogDisplay(logEntries)
	}

	lt.updateStatus(fmt.Sprintf("Loaded %d CloudWatch log entries from %d streams", len(logEntries), len(streams)), "green")

	// Start tailing if not already active
	lt.startTailing(logGroupName, streams)
}

// startTailing starts real-time tailing of log streams
func (lt *LogsTab) startTailing(logGroupName string, streams []clients.LogStreamInfo) {
	lt.mu.Lock()
	if lt.tailingActive {
		lt.mu.Unlock()
		return
	}

	// Cancel existing tailing if any
	if lt.cloudWatchCancel != nil {
		lt.cloudWatchCancel()
	}

	lt.cloudWatchCtx, lt.cloudWatchCancel = context.WithCancel(context.Background())
	lt.tailingActive = true
	lt.mu.Unlock()

	go func() {
		defer func() {
			lt.mu.Lock()
			lt.tailingActive = false
			lt.mu.Unlock()
		}()

		cloudWatchService := lt.awsClient.GetCloudWatchLogsService()
		if cloudWatchService == nil {
			return
		}

		// Extract stream names
		var streamNames []string
		for _, stream := range streams {
			streamNames = append(streamNames, stream.LogStreamName)
		}

		eventsChan := make(chan clients.LogEvent, 100)
		errorChan := make(chan error, 10)

		// Start tailing
		go cloudWatchService.TailLogStreams(lt.cloudWatchCtx, logGroupName, streamNames, eventsChan, errorChan)

		for {
			select {
			case <-lt.cloudWatchCtx.Done():
				return
			case event := <-eventsChan:
				lt.addCloudWatchEvent(event)
			case err := <-errorChan:
				logger.Error("CloudWatch tailing error", zap.Error(err))
				lt.updateStatus(fmt.Sprintf("Tailing error: %s", err.Error()), "red")
			}
		}
	}()
}

// addCloudWatchEvent adds a CloudWatch event to the logs
func (lt *LogsTab) addCloudWatchEvent(event clients.LogEvent) {
	entry := LogEntry{
		Level:   "INFO",
		Message: event.Message,
		Source:  "cloudwatch",
		Fields:  make(map[string]interface{}),
	}

	if event.Timestamp != 0 {
		entry.Timestamp = time.UnixMilli(event.Timestamp)
	} else {
		entry.Timestamp = time.Now()
	}

	if event.IngestionTime != 0 {
		entry.Fields["ingestionTime"] = time.UnixMilli(event.IngestionTime).Format("2006-01-02 15:04:05")
	}

	lt.addLogEntry("cloudwatch", entry)
}

// stopTailing stops the active tailing process
func (lt *LogsTab) stopTailing() {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if lt.cloudWatchCancel != nil {
		lt.cloudWatchCancel()
		lt.cloudWatchCancel = nil
	}
	lt.tailingActive = false
}

func (lt *LogsTab) GetLogCount(source string) int {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	if logs, exists := lt.logs[source]; exists {
		return len(logs)
	}
	return 0
}

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

// Cleanup stops any active tailing processes
func (lt *LogsTab) Cleanup() {
	lt.stopTailing()
}
