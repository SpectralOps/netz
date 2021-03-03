package cloud

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	log "github.com/cmpxchg16/netz/logger"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
)

const (
	defaultLogTimeout      = time.Minute * 120
	defaultLogPollInterval = time.Second * 5
)

type cloudwatchLogsInterface interface {
	DescribeLogStreamsPages(input *cloudwatchlogs.DescribeLogStreamsInput,
		fn func(*cloudwatchlogs.DescribeLogStreamsOutput, bool) bool) error
	DescribeLogStreams(input *cloudwatchlogs.DescribeLogStreamsInput) (*cloudwatchlogs.DescribeLogStreamsOutput, error)
	PutLogEvents(input *cloudwatchlogs.PutLogEventsInput) (*cloudwatchlogs.PutLogEventsOutput, error)
	FilterLogEventsPages(input *cloudwatchlogs.FilterLogEventsInput,
		fn func(*cloudwatchlogs.FilterLogEventsOutput, bool) bool) error
}

type logWaiter struct {
	CloudWatchLogs cloudwatchLogsInterface

	LogGroupName  string
	LogStreamName string

	Interval time.Duration
	Timeout  time.Duration
}

// streamExists checks the log group for a specific log stream
func (lw *logWaiter) streamExists() (bool, error) {
	params := &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName:        aws.String(lw.LogGroupName),
		LogStreamNamePrefix: aws.String(lw.LogStreamName),
		Descending:          aws.Bool(true),
	}

	var exists bool
	err := lw.CloudWatchLogs.DescribeLogStreamsPages(params,
		func(page *cloudwatchlogs.DescribeLogStreamsOutput, lastPage bool) bool {
			for _, stream := range page.LogStreams {
				// return early if we match the log stream
				if *stream.LogStreamName == lw.LogStreamName {
					exists = true
					return true
				}
			}
			return lastPage
		})

	return exists, err
}

// Wait waits for a log stream to exist
func (lw *logWaiter) Wait(ctx context.Context) error {
	log.Logger.Infof("waiting for log stream %s to exist...", lw.LogStreamName)
	t := time.Now()

	pollInterval := lw.Interval
	if pollInterval == time.Duration(0) {
		pollInterval = defaultLogPollInterval
	}

	timeout := lw.Timeout
	if timeout == time.Duration(0) {
		timeout = defaultLogTimeout
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	done := make(chan bool)
	go func() {
		time.Sleep(timeout)
		done <- true
	}()

	for {
		exists, err := lw.streamExists()

		// handle rate-limiting errors which seem to occur during
		// excessive polling operations
		if isRateLimited(err) {
			time.Sleep(5 * time.Second)
			continue
		} else if err != nil {
			//return err
			continue
		} else if exists {
			log.Logger.Infof("found stream %s after %v", lw.LogStreamName, time.Now().Sub(t))
			return nil
		}

		select {
		case <-done:
			log.Logger.Error("timed out waiting for stream")
			return fmt.Errorf("timed out waiting for stream %s", lw.LogStreamName)
		case <-ticker.C:
			continue
		}
	}
}

func isRateLimited(err error) bool {
	if aerr, ok := err.(awserr.Error); ok {
		if aerr.Code() == "Throttling" {
			return true
		}
	}
	return false
}

type logWatcher struct {
	CloudWatchLogs cloudwatchLogsInterface

	LogGroupName  string
	LogStreamName string
	Printer       func(event *cloudwatchlogs.FilteredLogEvent) bool

	Interval time.Duration
	Timeout  time.Duration

	mu   sync.Mutex
	stop chan struct{}
}

func (lw *logWatcher) Watch(ctx context.Context) error {
	lw.mu.Lock()
	lw.stop = make(chan struct{})
	lw.mu.Unlock()

	waiter := &logWaiter{
		CloudWatchLogs: lw.CloudWatchLogs,
		LogGroupName:   lw.LogGroupName,
		LogStreamName:  lw.LogStreamName,
		Interval:       lw.Interval,
		Timeout:        lw.Timeout,
	}

	if err := waiter.Wait(ctx); err != nil {
		return err
	}

	var after int64
	var err error

	pollInterval := lw.Interval
	if pollInterval == time.Duration(0) {
		pollInterval = time.Second * 5
	}

	for {
		select {
		case <-time.After(pollInterval):
			if after, err = lw.printEventsAfter(ctx, after); err != nil {
				return err
			}

		case <-lw.stop:
			return nil

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (lw *logWatcher) Stop() error {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	if lw.stop != nil {
		close(lw.stop)
		return nil
	}
	return errors.New("log watcher not started")
}

func (lw *logWatcher) printEventsAfter(ctx context.Context, ts int64) (int64, error) {
	log.Logger.Tracef("Printing events in stream %q after %d", lw.LogStreamName, ts)
	t := time.Now()
	var count int64

	filterInput := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName:   aws.String(lw.LogGroupName),
		LogStreamNames: aws.StringSlice([]string{lw.LogStreamName}),
		StartTime:      aws.Int64(ts + 1),
	}

	err := lw.CloudWatchLogs.FilterLogEventsPages(filterInput,
		func(p *cloudwatchlogs.FilterLogEventsOutput, lastPage bool) (shouldContinue bool) {
			for _, event := range p.Events {
				log.Logger.Trace(event)
				count++
				if !lw.Printer(event) {
					log.Logger.Info("stopping log watcher via print function")
					lw.Stop()
				}
				if *event.Timestamp > ts {
					ts = *event.Timestamp
				}
			}
			return lastPage
		})
	if err != nil {
		log.Logger.Tracef("printed %d events in %v", count, time.Now().Sub(t))
	}

	return ts, err
}

type logWriter struct {
	CloudWatchLogs cloudwatchLogsInterface

	LogGroupName  string
	LogStreamName string

	Interval time.Duration
	Timeout  time.Duration
}

func (lw *logWriter) nextSequenceToken() (*string, error) {
	log.Logger.Tracef("finding next sequence token for stream %s", lw.LogStreamName)

	streams, err := lw.CloudWatchLogs.DescribeLogStreams(&cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName:        aws.String(lw.LogGroupName),
		LogStreamNamePrefix: aws.String(lw.LogStreamName),
		Descending:          aws.Bool(true),
		Limit:               aws.Int64(1),
	})
	if err != nil {
		return nil, err
	} else if len(streams.LogStreams) == 0 {
		return nil, fmt.Errorf("failed to find stream %s in group %s", lw.LogStreamName, lw.LogGroupName)
	}

	return streams.LogStreams[0].UploadSequenceToken, nil
}

func (lw *logWriter) WriteString(ctx context.Context, msg string) error {
	waiter := &logWaiter{
		CloudWatchLogs: lw.CloudWatchLogs,
		LogGroupName:   lw.LogGroupName,
		LogStreamName:  lw.LogStreamName,
		Interval:       lw.Interval,
		Timeout:        lw.Timeout,
	}

	if err := waiter.Wait(ctx); err != nil {
		return err
	}

	sequence, err := lw.nextSequenceToken()
	if err != nil {
		return err
	}

	log.Logger.Tracef("putting log message %q to %s", msg, lw.LogStreamName)
	_, err = lw.CloudWatchLogs.PutLogEvents(&cloudwatchlogs.PutLogEventsInput{
		SequenceToken: sequence,
		LogGroupName:  aws.String(lw.LogGroupName),
		LogStreamName: aws.String(lw.LogStreamName),
		LogEvents: []*cloudwatchlogs.InputLogEvent{
			{
				Message:   aws.String(msg),
				Timestamp: aws.Int64(aws.TimeUnixMilli(time.Now())),
			},
		},
	})
	return err
}

func createLogGroup(sess *session.Session, logGroup string) error {
	cwl := cloudwatchlogs.New(sess)
	groups, err := cwl.DescribeLogGroups(&cloudwatchlogs.DescribeLogGroupsInput{
		Limit:              aws.Int64(1),
		LogGroupNamePrefix: aws.String(logGroup),
	})
	if err != nil {
		return err
	}
	if len(groups.LogGroups) == 0 {
		log.Logger.Infof("creating log group %s", logGroup)
		_, err = cwl.CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(logGroup),
		})
		if err != nil {
			return err
		}
	} else {
		log.Logger.Debugf("log group %s exists", logGroup)
	}
	return nil
}
