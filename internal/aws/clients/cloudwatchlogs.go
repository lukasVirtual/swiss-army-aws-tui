package clients

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// CloudWatchLogsService wraps the CloudWatch Logs client
type CloudWatchLogsService struct {
	client *cloudwatchlogs.Client
}

// LogStreamInfo represents information about a log stream
type LogStreamInfo struct {
	LogStreamName       string
	FirstEventTime      int64
	LastEventTime       int64
	LastIngestionTime   int64
	UploadSequenceToken *string
	Arn                 string
}

// LogEvent represents a single log event
type LogEvent struct {
	Timestamp     int64
	Message       string
	IngestionTime int64
}

// NewCloudWatchLogsService creates a new CloudWatch Logs service wrapper
func NewCloudWatchLogsService(client *cloudwatchlogs.Client) (*CloudWatchLogsService, error) {
	if client == nil {
		return nil, fmt.Errorf("CloudWatch Logs client not provided")
	}

	return &CloudWatchLogsService{
		client: client,
	}, nil
}

// DescribeLogStreams retrieves log streams for a given log group
func (s *CloudWatchLogsService) DescribeLogStreams(ctx context.Context, logGroupName string, limit int32) ([]LogStreamInfo, error) {
	descending := true

	input := &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: &logGroupName,
		OrderBy:      types.OrderByLastEventTime,
		Descending:   &descending,
	}

	if limit > 0 {
		input.Limit = &limit
	}

	result, err := s.client.DescribeLogStreams(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe log streams: %w", err)
	}

	var streams []LogStreamInfo
	for _, stream := range result.LogStreams {
		streamInfo := LogStreamInfo{
			LogStreamName: *stream.LogStreamName,
			Arn:           *stream.Arn,
		}

		if stream.FirstEventTimestamp != nil {
			streamInfo.FirstEventTime = *stream.FirstEventTimestamp
		}
		if stream.LastEventTimestamp != nil {
			streamInfo.LastEventTime = *stream.LastEventTimestamp
		}
		if stream.LastIngestionTime != nil {
			streamInfo.LastIngestionTime = *stream.LastIngestionTime
		}
		if stream.UploadSequenceToken != nil {
			streamInfo.UploadSequenceToken = stream.UploadSequenceToken
		}

		streams = append(streams, streamInfo)
	}

	return streams, nil
}

// GetLogEvents retrieves log events from a specific log stream
func (s *CloudWatchLogsService) GetLogEvents(ctx context.Context, logGroupName, logStreamName string, limit int32, startFromHead bool) ([]LogEvent, *string, error) {
	input := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  &logGroupName,
		LogStreamName: &logStreamName,
		StartFromHead: &startFromHead,
	}

	if limit > 0 {
		input.Limit = &limit
	}

	result, err := s.client.GetLogEvents(ctx, input)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get log events: %w", err)
	}

	var events []LogEvent
	for _, event := range result.Events {
		logEvent := LogEvent{
			Message: *event.Message,
		}

		if event.Timestamp != nil {
			logEvent.Timestamp = *event.Timestamp
		}
		if event.IngestionTime != nil {
			logEvent.IngestionTime = *event.IngestionTime
		}

		events = append(events, logEvent)
	}

	var nextForwardToken *string
	if result.NextForwardToken != nil {
		nextForwardToken = result.NextForwardToken
	}

	return events, nextForwardToken, nil
}

// GetLogEventsWithToken retrieves log events using a pagination token
func (s *CloudWatchLogsService) GetLogEventsWithToken(ctx context.Context, logGroupName, logStreamName, nextToken string, limit int32) ([]LogEvent, *string, error) {
	input := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  &logGroupName,
		LogStreamName: &logStreamName,
		NextToken:     &nextToken,
	}

	if limit > 0 {
		input.Limit = &limit
	}

	result, err := s.client.GetLogEvents(ctx, input)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get log events with token: %w", err)
	}

	var events []LogEvent
	for _, event := range result.Events {
		logEvent := LogEvent{
			Message: *event.Message,
		}

		if event.Timestamp != nil {
			logEvent.Timestamp = *event.Timestamp
		}
		if event.IngestionTime != nil {
			logEvent.IngestionTime = *event.IngestionTime
		}

		events = append(events, logEvent)
	}

	var nextForwardToken *string
	if result.NextForwardToken != nil {
		nextForwardToken = result.NextForwardToken
	}

	return events, nextForwardToken, nil
}

// GetLogEventsSinceTime retrieves log events since a specific timestamp
func (s *CloudWatchLogsService) GetLogEventsSinceTime(ctx context.Context, logGroupName, logStreamName string, since time.Time, limit int32) ([]LogEvent, error) {
	sinceTime := since.UnixMilli()
	startFromHead := false

	input := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  &logGroupName,
		LogStreamName: &logStreamName,
		StartTime:     &sinceTime,
		StartFromHead: &startFromHead,
	}

	if limit > 0 {
		input.Limit = &limit
	}

	result, err := s.client.GetLogEvents(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get log events since time: %w", err)
	}

	var events []LogEvent
	for _, event := range result.Events {
		logEvent := LogEvent{
			Message: *event.Message,
		}

		if event.Timestamp != nil {
			logEvent.Timestamp = *event.Timestamp
		}
		if event.IngestionTime != nil {
			logEvent.IngestionTime = *event.IngestionTime
		}

		events = append(events, logEvent)
	}

	return events, nil
}

// TailLogStreams tails multiple log streams in real-time
func (s *CloudWatchLogsService) TailLogStreams(ctx context.Context, logGroupName string, logStreamNames []string, eventsChan chan<- LogEvent, errorChan chan<- error) {
	defer close(eventsChan)
	defer close(errorChan)

	// Track the next token for each stream
	nextTokens := make(map[string]*string)

	// Initialize tokens for all streams
	for _, streamName := range logStreamNames {
		events, nextToken, err := s.GetLogEvents(ctx, logGroupName, streamName, 10, false)
		if err != nil {
			errorChan <- fmt.Errorf("failed to get initial events for stream %s: %w", streamName, err)
			continue
		}

		for _, event := range events {
			select {
			case eventsChan <- event:
			case <-ctx.Done():
				return
			}
		}

		if nextToken != nil {
			nextTokens[streamName] = nextToken
		}
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Poll for new events in each stream
			for _, streamName := range logStreamNames {
				if nextToken, exists := nextTokens[streamName]; exists && nextToken != nil {
					events, newNextToken, err := s.GetLogEventsWithToken(ctx, logGroupName, streamName, *nextToken, 50)
					if err != nil {
						errorChan <- fmt.Errorf("failed to tail events for stream %s: %w", streamName, err)
						continue
					}

					for _, event := range events {
						select {
						case eventsChan <- event:
						case <-ctx.Done():
							return
						}
					}

					if newNextToken != nil {
						nextTokens[streamName] = newNextToken
					}
				}
			}
		}
	}
}

func (s *CloudWatchLogsService) ListAllLogGroups(ctx context.Context) ([]types.LogGroupSummary, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("CloudWatch Logs service not initialized")
	}

	input := &cloudwatchlogs.ListLogGroupsInput{}
	result, err := s.client.ListLogGroups(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list log groups: %w", err)
	}
	return result.LogGroups, nil
}
