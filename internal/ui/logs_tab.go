package ui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"swiss-army-tui/internal/aws"
	"swiss-army-tui/internal/aws/clients"
	"swiss-army-tui/pkg/logger"

	"github.com/blevesearch/bleve/v2"
	blevequery "github.com/blevesearch/bleve/v2/search/query"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"go.uber.org/zap"
)

type LogsTab struct {
	view *tview.Flex
	app  *tview.Application

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

	// Bleve search index
	searchIndex   bleve.Index
	searchIndexMu sync.RWMutex
}

type LogEntry struct {
	Timestamp  time.Time
	Level      string
	Message    string
	Source     string
	Fields     map[string]interface{}
	Highlights map[string][]string // Store highlighting information
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

func NewLogsTab(app *tview.Application) (*LogsTab, error) {
	tab := &LogsTab{
		app:        app,
		logs:       make(map[string][]LogEntry),
		autoScroll: true,
		maxLines:   1000,
	}

	// Initialize Bleve search index
	if err := tab.initializeSearchIndex(); err != nil {
		logger.Warn("Failed to initialize search index", zap.Error(err))
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

	lt.filterInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			if lt.app != nil {
				lt.app.SetFocus(lt.logSourceList)
			}
			lt.filterInput.SetBorder(true).SetTitle(" Filter Logs ").SetTitleAlign(tview.AlignLeft)
			return nil
		case tcell.KeyEnter:
			if lt.app != nil {
				lt.app.SetFocus(lt.logSourceList)
			}
			return nil
		}
		return event
	})

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

		// Apply highlighting to the message
		highlightedMessage := log.Message
		if log.Highlights != nil && len(log.Highlights["Message"]) > 0 {
			highlightedMessage = lt.renderHighlightedText(log.Message, "", log.Highlights["Message"])
		} else if filterText != "" {
			highlightedMessage = lt.renderHighlightedText(log.Message, filterText, nil)
		}

		// Apply highlighting to the level if needed
		highlightedLevel := strings.ToUpper(log.Level)
		if log.Highlights != nil && len(log.Highlights["Level"]) > 0 {
			highlightedLevel = lt.renderHighlightedText(highlightedLevel, "", log.Highlights["Level"])
		} else if filterText != "" && strings.Contains(strings.ToLower(log.Level), filterText) {
			highlightedLevel = lt.renderHighlightedText(highlightedLevel, filterText, nil)
		}

		logText.WriteString(fmt.Sprintf("[gray]%s[-] [%s]%-5s[-] %s\n",
			timestamp, levelColor, highlightedLevel, highlightedMessage))

		if len(log.Fields) > 0 {
			var fieldKeys []string
			for key := range log.Fields {
				fieldKeys = append(fieldKeys, key)
			}
			sort.Strings(fieldKeys)

			for _, key := range fieldKeys {
				fieldValue := fmt.Sprintf("%v", log.Fields[key])
				if filterText != "" && strings.Contains(strings.ToLower(fieldValue), filterText) {
					fieldValue = lt.renderHighlightedText(fieldValue, filterText, nil)
				}
				logText.WriteString(fmt.Sprintf("  [blue]%s:[-] %s\n", key, fieldValue))
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
	// Check if this looks like a search query (contains advanced operators)
	if strings.Contains(text, " ") || strings.Contains(text, "\"") || strings.Contains(text, "*") {
		lt.performSearch(text)
	} else {
		lt.applyFilter()
	}
}

// initializeSearchIndex creates a Bleve index for fast log searching
func (lt *LogsTab) initializeSearchIndex() error {
	// Create a memory-based index for now (could be persisted later)
	mapping := bleve.NewIndexMapping()

	logEntryMapping := bleve.NewDocumentMapping()

	messageFieldMapping := bleve.NewTextFieldMapping()
	messageFieldMapping.Analyzer = "standard"
	messageFieldMapping.Store = true
	messageFieldMapping.Index = true
	messageFieldMapping.IncludeTermVectors = true
	logEntryMapping.AddFieldMappingsAt("Message", messageFieldMapping)

	levelFieldMapping := bleve.NewTextFieldMapping()
	levelFieldMapping.Analyzer = "keyword"
	levelFieldMapping.Store = true
	levelFieldMapping.Index = true
	levelFieldMapping.IncludeTermVectors = true
	logEntryMapping.AddFieldMappingsAt("Level", levelFieldMapping)

	sourceFieldMapping := bleve.NewTextFieldMapping()
	sourceFieldMapping.Analyzer = "keyword"
	sourceFieldMapping.Store = true
	sourceFieldMapping.Index = true
	sourceFieldMapping.IncludeTermVectors = true
	logEntryMapping.AddFieldMappingsAt("Source", sourceFieldMapping)

	timestampFieldMapping := bleve.NewNumericFieldMapping()
	timestampFieldMapping.Store = true
	timestampFieldMapping.Index = true
	logEntryMapping.AddFieldMappingsAt("Timestamp", timestampFieldMapping)

	mapping.AddDocumentMapping("_default", logEntryMapping)

	// Create index in memory
	index, err := bleve.NewMemOnly(mapping)
	if err != nil {
		return fmt.Errorf("failed to create search index: %w", err)
	}

	lt.searchIndexMu.Lock()
	lt.searchIndex = index
	lt.searchIndexMu.Unlock()

	logger.Info("Search index initialized successfully")
	return nil
}

// indexLogEntry adds a log entry to the search index
func (lt *LogsTab) indexLogEntry(entry LogEntry) {
	lt.searchIndexMu.RLock()
	index := lt.searchIndex
	lt.searchIndexMu.RUnlock()

	if index == nil {
		return
	}

	// Create a unique ID for the entry
	id := fmt.Sprintf("%s_%d_%s", entry.Source, entry.Timestamp.UnixNano(), entry.Message[:min(50, len(entry.Message))])

	// Index the document
	err := index.Index(id, entry)
	if err != nil {
		logger.Debug("Failed to index log entry", zap.Error(err))
	}
}

// performSearch executes a search query using Bleve
func (lt *LogsTab) performSearch(queryStr string) {
	lt.searchIndexMu.RLock()
	index := lt.searchIndex
	lt.searchIndexMu.RUnlock()

	if index == nil {
		lt.updateStatus("Search index not available", "red")
		return
	}

	// Create a query based on the input
	query := lt.buildSearchQuery(queryStr)
	if query == nil {
		lt.applyFilter() // Fallback to basic filter
		return
	}

	searchRequest := bleve.NewSearchRequest(query)
	searchRequest.Size = 1000    // Limit results
	searchRequest.Explain = true // Enable explanations for debugging

	// Enhance search request for better highlighting
	lt.enhanceSearchRequest(searchRequest)

	// Execute search
	searchResults, err := index.Search(searchRequest)
	if err != nil {
		logger.Error("Search failed", zap.Error(err))
		lt.updateStatus(fmt.Sprintf("Search error: %s", err.Error()), "red")
		return
	}

	lt.mu.Lock()
	defer lt.mu.Unlock()

	var searchResultsEntries []LogEntry
	for _, hit := range searchResults.Hits {
		// Try to find the original log entry
		if entry := lt.findLogEntryByID(hit.ID); entry != nil {
			// Create a copy with highlighting information
			highlightedEntry := *entry
			highlightedEntry.Highlights = make(map[string][]string)

			// Extract highlighting information from Bleve
			for field, fragments := range hit.Fragments {
				highlightedEntry.Highlights[field] = fragments
			}

			searchResultsEntries = append(searchResultsEntries, highlightedEntry)
		}
	}

	// Update display with search results
	lt.filteredLogs = searchResultsEntries
	lt.updateLogDisplayFromFiltered()

	// Update status
	status := fmt.Sprintf("Found %d results for '%s'", len(searchResultsEntries), queryStr)
	lt.updateStatus(status, "green")
}

func (lt *LogsTab) renderHighlightedText(text, searchTerm string, highlights []string) string {
	if searchTerm == "" && len(highlights) == 0 {
		return text
	}

	if len(highlights) > 0 {
		return lt.renderBleveHighlights(text, highlights)
	}

	if searchTerm != "" {
		return lt.renderSimpleHighlight(text, searchTerm)
	}

	return text
}

func (lt *LogsTab) renderBleveHighlights(text string, highlights []string) string {
	for _, fragment := range highlights {
		if strings.Contains(fragment, "\x1b[") {
			// Convert ANSI escape sequences to tview format
			return lt.convertAnsiToTview(fragment)
		}
	}

	result := text
	for _, fragment := range highlights {
		// Replace Bleve's <mark> tags with tview highlighting
		highlighted := strings.ReplaceAll(fragment, "<mark>", "[#ffff00::b]")
		highlighted = strings.ReplaceAll(highlighted, "</mark>", "[-]")
		result = highlighted
	}
	return result
}

func (lt *LogsTab) renderSimpleHighlight(text, searchTerm string) string {
	lowerText := strings.ToLower(text)
	lowerSearch := strings.ToLower(searchTerm)

	var result strings.Builder
	start := 0

	for {
		index := strings.Index(lowerText[start:], lowerSearch)
		if index == -1 {
			result.WriteString(text[start:])
			break
		}

		result.WriteString(text[start : start+index])

		matchStart := start + index
		matchEnd := matchStart + len(searchTerm)
		result.WriteString(fmt.Sprintf("[#ffff00::b]%s[-]", text[matchStart:matchEnd]))

		start = matchEnd
	}

	return result.String()
}

func (lt *LogsTab) convertAnsiToTview(text string) string {
	ansiToTview := map[string]string{
		"\x1b[31m":   "[red]",        // Red
		"\x1b[33m":   "[yellow]",     // Yellow
		"\x1b[33;1m": "[#ffff00::b]", // Bold Yellow (highlight)
		"\x1b[43m":   "[#ffff00]",    // Yellow background
		"\x1b[1m":    "[::b]",        // Bold
		"\x1b[0m":    "[-]",          // Reset
	}

	result := text
	for ansi, tview := range ansiToTview {
		result = strings.ReplaceAll(result, ansi, tview)
	}

	result = strings.ReplaceAll(result, "\x1b[", "[")
	result = strings.ReplaceAll(result, "m", "]")

	return result
}

func (lt *LogsTab) enhanceSearchRequest(searchRequest *bleve.SearchRequest) {
	if searchRequest.Highlight == nil {
		searchRequest.Highlight = bleve.NewHighlight()
	}

	if len(searchRequest.Fields) == 0 {
		searchRequest.Fields = []string{"*"}
	}
}

func (lt *LogsTab) buildSearchQuery(queryStr string) blevequery.Query {
	if strings.TrimSpace(queryStr) == "" {
		return nil
	}

	if strings.Contains(queryStr, " ") || strings.Contains(queryStr, "\"") || strings.Contains(queryStr, "*") {
		query := blevequery.NewQueryStringQuery(queryStr)
		return query
	}

	messageQuery := blevequery.NewMatchQuery(queryStr)
	messageQuery.SetField("Message")
	messageQuery.SetBoost(1.5) // Boost message field matches

	levelQuery := blevequery.NewMatchQuery(queryStr)
	levelQuery.SetField("Level")

	sourceQuery := blevequery.NewMatchQuery(queryStr)
	sourceQuery.SetField("Source")

	return blevequery.NewDisjunctionQuery([]blevequery.Query{
		messageQuery,
		levelQuery,
		sourceQuery,
	})
}

// findLogEntryByID finds a log entry by its unique ID
func (lt *LogsTab) findLogEntryByID(id string) *LogEntry {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	// Parse ID to extract timestamp and source
	parts := strings.Split(id, "_")
	if len(parts) < 3 {
		return nil
	}

	source := parts[0]
	var timestamp time.Time
	if len(parts) > 1 {
		if parsed, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			timestamp = time.Unix(0, parsed)
		}
	}

	// Find the entry in the logs
	for _, entries := range lt.logs {
		for _, entry := range entries {
			if entry.Source == source && entry.Timestamp.Equal(timestamp) {
				// Check if message matches (partial match for first 50 chars)
				if len(entry.Message) > 50 && len(parts) >= 3 {
					if entry.Message[:50] == strings.Join(parts[2:], "_") {
						return &entry
					}
				} else if entry.Message == strings.Join(parts[2:], "_") {
					return &entry
				}
			}
		}
	}

	return nil
}

// updateLogDisplayFromFiltered updates the display using filteredLogs
func (lt *LogsTab) updateLogDisplayFromFiltered() {
	if lt.logView == nil {
		return
	}
	lt.logView.Clear()

	var logText strings.Builder
	filterText := strings.ToLower(strings.TrimSpace(lt.filterInput.GetText()))

	for _, log := range lt.filteredLogs {
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

		highlightedMessage := log.Message
		if log.Highlights != nil && len(log.Highlights["Message"]) > 0 {
			highlightedMessage = lt.renderHighlightedText(log.Message, "", log.Highlights["Message"])
		} else if filterText != "" {
			highlightedMessage = lt.renderHighlightedText(log.Message, filterText, nil)
		}

		highlightedLevel := strings.ToUpper(log.Level)
		if log.Highlights != nil && len(log.Highlights["Level"]) > 0 {
			highlightedLevel = lt.renderHighlightedText(highlightedLevel, "", log.Highlights["Level"])
		} else if filterText != "" && strings.Contains(strings.ToLower(log.Level), filterText) {
			highlightedLevel = lt.renderHighlightedText(highlightedLevel, filterText, nil)
		}

		logText.WriteString(fmt.Sprintf("[gray]%s[-] [%s]%-5s[-] %s\n",
			timestamp, levelColor, highlightedLevel, highlightedMessage))

		if len(log.Fields) > 0 {
			var fieldKeys []string
			for key := range log.Fields {
				fieldKeys = append(fieldKeys, key)
			}
			sort.Strings(fieldKeys)

			for _, key := range fieldKeys {
				fieldValue := fmt.Sprintf("%v", log.Fields[key])
				if log.Highlights != nil && len(log.Highlights[key]) > 0 {
					fieldValue = lt.renderHighlightedText(fieldValue, "", log.Highlights[key])
				} else if filterText != "" && strings.Contains(strings.ToLower(fieldValue), filterText) {
					fieldValue = lt.renderHighlightedText(fieldValue, filterText, nil)
				}
				logText.WriteString(fmt.Sprintf("  [blue]%s:[-] %s\n", key, fieldValue))
			}
		}
	}

	lt.logView.SetText(logText.String())

	if lt.autoScroll {
		lt.logView.ScrollToEnd()
	}

	title := fmt.Sprintf(" Logs (%d", len(lt.filteredLogs))
	if len(lt.filteredLogs) != len(lt.logs[lt.selectedSource]) {
		title += fmt.Sprintf(" of %d", len(lt.logs[lt.selectedSource]))
	}
	title += ") "
	lt.logView.SetTitle(title)
}

func (lt *LogsTab) addLogEntry(sourceName string, entry LogEntry) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if lt.logs[sourceName] == nil {
		lt.logs[sourceName] = []LogEntry{}
	}

	// Add to logs
	lt.logs[sourceName] = append(lt.logs[sourceName], entry)

	// Index the entry for fast search
	go lt.indexLogEntry(entry)

	// Update display if this is the current source
	if sourceName == lt.selectedSource {
		lt.applyFilter()
	}

	if len(lt.logs[sourceName]) > lt.maxLines {
		lt.logs[sourceName] = lt.logs[sourceName][len(lt.logs[sourceName])-lt.maxLines:]
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
			Level:     "DEBUG",
			Message:   "Loading configuration from config.yaml",
			Source:    "app",
			Fields:    map[string]interface{}{"config_file": "config.yaml"},
		},
		{
			Timestamp: time.Now().Add(-3 * time.Minute),
			Level:     "WARN",
			Message:   "Deprecated API endpoint used",
			Source:    "app",
			Fields:    map[string]interface{}{"endpoint": "/old-api", "replacement": "/new-api"},
		},
		{
			Timestamp: time.Now().Add(-2 * time.Minute),
			Level:     "ERROR",
			Message:   "Database connection failed",
			Source:    "app",
			Fields:    map[string]interface{}{"error": "connection timeout", "retry_count": 3},
		},
		{
			Timestamp: time.Now().Add(-1 * time.Minute),
			Level:     "INFO",
			Message:   "Database connection re-established",
			Source:    "app",
			Fields:    map[string]interface{}{"connection_time": "150ms"},
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
	if lt.filterInput != nil && lt.app != nil {
		lt.app.SetFocus(lt.filterInput)
		lt.filterInput.SetBorder(true).SetTitle(" Filter Logs (Active) ").SetTitleAlign(tview.AlignLeft)
	}
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

	streams, err := cloudWatchService.DescribeLogStreams(ctx, logGroupName, 10)
	if err != nil {
		logger.Error("Failed to describe log streams", zap.String("logGroup", logGroupName), zap.Error(err))
		if lt.app != nil {
			lt.app.QueueUpdateDraw(func() {
				lt.updateStatus(fmt.Sprintf("Failed to get log streams: %s", err.Error()), "red")
			})
		}
		return
	}

	if len(streams) == 0 {
		if lt.app != nil {
			lt.app.QueueUpdateDraw(func() {
				lt.updateStatus(fmt.Sprintf("No log streams found in %s", logGroupName), "yellow")
			})
		}
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

	sort.Slice(logEntries, func(i, j int) bool {
		return logEntries[i].Timestamp.After(logEntries[j].Timestamp)
	})

	lt.logs["cloudwatch"] = logEntries
	lt.mu.Unlock()

	lt.mu.RLock()
	selectedSource := lt.selectedSource
	lt.mu.RUnlock()

	if selectedSource == "cloudwatch" {
		if lt.app != nil {
			lt.app.QueueUpdateDraw(func() {
				lt.updateLogDisplay(logEntries)
			})
		}
	}

	if lt.app != nil {
		lt.app.QueueUpdateDraw(func() {
			lt.updateStatus(fmt.Sprintf("Loaded %d CloudWatch log entries from %d streams", len(logEntries), len(streams)), "green")
		})
	}

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

		var streamNames []string
		for _, stream := range streams {
			streamNames = append(streamNames, stream.LogStreamName)
		}

		eventsChan := make(chan clients.LogEvent, 100)
		errorChan := make(chan error, 10)

		go cloudWatchService.TailLogStreams(lt.cloudWatchCtx, logGroupName, streamNames, eventsChan, errorChan)

		for {
			select {
			case <-lt.cloudWatchCtx.Done():
				return
			case event := <-eventsChan:
				lt.addCloudWatchEvent(event)
			case err := <-errorChan:
				logger.Error("CloudWatch tailing error", zap.Error(err))
				if lt.app != nil {
					lt.app.QueueUpdateDraw(func() {
						lt.updateStatus(fmt.Sprintf("Tailing error: %s", err.Error()), "red")
					})
				}
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

	if lt.app != nil {
		lt.app.QueueUpdateDraw(func() {
			lt.addLogEntry("cloudwatch", entry)
		})
	} else {
		lt.addLogEntry("cloudwatch", entry)
	}
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
			if _, err := writer.WriteString(line); err != nil {
				return fmt.Errorf("failed to write log line: %w", err)
			}
		}
	}

	return nil
}

// Cleanup stops any active tailing processes and closes the search index
func (lt *LogsTab) Cleanup() {
	lt.stopTailing()

	lt.searchIndexMu.Lock()
	defer lt.searchIndexMu.Unlock()

	if lt.searchIndex != nil {
		lt.searchIndex.Close()
		lt.searchIndex = nil
		logger.Info("Search index closed")
	}
}
