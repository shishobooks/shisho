package joblogs

import (
	"context"
	"runtime/debug"

	"github.com/robinjoseph08/golib/logger"
	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/models"
)

const maxDataValueLen = 1024

// JobLogger wraps logging to both stdout and database.
type JobLogger struct {
	jobID   int
	service *Service
	log     logger.Logger
	ctx     context.Context
}

// NewJobLogger creates a new JobLogger for a specific job.
func (svc *Service) NewJobLogger(ctx context.Context, jobID int, log logger.Logger) *JobLogger {
	return &JobLogger{
		jobID:   jobID,
		service: svc,
		log:     log.Data(logger.Data{"job_id": jobID}),
		ctx:     ctx,
	}
}

// Info logs an info-level message.
func (l *JobLogger) Info(msg string, data logger.Data) {
	l.log.Info(msg, data)
	l.persist(models.JobLogLevelInfo, msg, data, nil)
}

// Warn logs a warning-level message.
func (l *JobLogger) Warn(msg string, data logger.Data) {
	l.log.Warn(msg, data)
	l.persist(models.JobLogLevelWarn, msg, data, nil)
}

// Error logs an error-level message with automatic stack trace.
func (l *JobLogger) Error(msg string, err error, data logger.Data) {
	l.log.Err(err).Error(msg, data)
	stack := string(debug.Stack())
	l.persist(models.JobLogLevelError, msg, data, &stack)
}

// Fatal logs a fatal-level message with automatic stack trace (for panics).
func (l *JobLogger) Fatal(msg string, err error, data logger.Data) {
	if data == nil {
		data = logger.Data{}
	}
	if err != nil {
		data["error"] = err.Error()
	}
	l.log.Error(msg, data)
	stack := string(debug.Stack())
	l.persist(models.JobLogLevelFatal, msg, data, &stack)
}

func (l *JobLogger) persist(level, msg string, data logger.Data, stackTrace *string) {
	// Extract "plugin" from data into dedicated column
	var plugin *string
	if len(data) > 0 {
		if p, ok := data["plugin"]; ok {
			if ps, ok := p.(string); ok && ps != "" {
				plugin = &ps
			}
		}
	}

	var dataStr *string
	if len(data) > 0 {
		truncatedData := make(logger.Data)
		for k, v := range data {
			if k == "plugin" {
				continue // stored in dedicated column
			}
			s, ok := v.(string)
			if ok && len(s) > maxDataValueLen {
				truncatedData[k] = truncateMiddle(s, maxDataValueLen)
			} else {
				truncatedData[k] = v
			}
		}
		if len(truncatedData) > 0 {
			jsonBytes, err := json.Marshal(truncatedData)
			if err == nil {
				s := string(jsonBytes)
				dataStr = &s
			}
		}
	}

	jobLog := &models.JobLog{
		JobID:      l.jobID,
		Level:      level,
		Message:    msg,
		Plugin:     plugin,
		Data:       dataStr,
		StackTrace: stackTrace,
	}

	_ = l.service.CreateJobLog(l.ctx, jobLog)
}

func truncateMiddle(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	half := (maxLen - 5) / 2
	return s[:half] + " ... " + s[len(s)-half:]
}
