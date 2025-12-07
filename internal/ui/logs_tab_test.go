package ui

import (
	"testing"
	"time"
)

func TestLogsTabHighlighting(t *testing.T) {
	lt := &LogsTab{
		logs:       make(map[string][]LogEntry),
		autoScroll: true,
		maxLines:   1000,
	}

	// Test simple highlighting
	text := "This is an error message in logs"
	searchTerm := "error"
	result := lt.renderSimpleHighlight(text, searchTerm)
	expected := "This is an [#ffff00::b]error[-] message in logs"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}

	// Test case-insensitive highlighting
	text2 := "ERROR: Something went wrong"
	searchTerm2 := "error"
	result2 := lt.renderSimpleHighlight(text2, searchTerm2)
	expected2 := "[#ffff00::b]ERROR[-]: Something went wrong"

	if result2 != expected2 {
		t.Errorf("Expected %q, got %q", expected2, result2)
	}

	// Test Bleve highlight conversion
	highlights := []string{"This is a <mark>highlighted</mark> text"}
	result3 := lt.renderBleveHighlights("This is a highlighted text", highlights)
	expected3 := "This is a [#ffff00::b]highlighted[-] text"

	if result3 != expected3 {
		t.Errorf("Expected %q, got %q", expected3, result3)
	}
}

func TestLogsTabSearchQuery(t *testing.T) {
	lt := &LogsTab{}

	// Test simple query
	query := lt.buildSearchQuery("error")
	if query == nil {
		t.Error("Expected non-nil query for simple search term")
	}

	// Test complex query
	query = lt.buildSearchQuery("error AND warning")
	if query == nil {
		t.Error("Expected non-nil query for complex search term")
	}

	// Test empty query
	query = lt.buildSearchQuery("")
	if query != nil {
		t.Error("Expected nil query for empty search term")
	}
}

func TestLogsTabAddEntry(t *testing.T) {
	lt := &LogsTab{
		logs:       make(map[string][]LogEntry),
		autoScroll: true,
		maxLines:   1000,
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   "Test message",
		Source:    "test",
		Fields:    map[string]interface{}{"key": "value"},
	}

	lt.addLogEntry("test", entry)

	if len(lt.logs["test"]) != 1 {
		t.Errorf("Expected 1 log entry, got %d", len(lt.logs["test"]))
	}

	if lt.logs["test"][0].Message != "Test message" {
		t.Errorf("Expected 'Test message', got %q", lt.logs["test"][0].Message)
	}
}

func TestLogsTabSetAWSClient(t *testing.T) {
	lt := &LogsTab{}

	// Test setting nil client
	lt.SetAWSClient(nil)
	if lt.awsClient != nil {
		t.Error("Expected awsClient to be nil")
	}

	// Note: We can't easily test with a real AWS client without AWS credentials
	// but we can verify method doesn't panic
	lt.SetAWSClient(nil)
}
