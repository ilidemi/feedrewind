package crawler

import (
	"fmt"
	"strings"
)

// The error can only be ErrCrawlCanceled
type ProgressSaver interface {
	SaveStatusAndCount(status string, maybeCount *int) error
	SaveStatus(status string) error
	SaveCount(maybeCount *int) error
	EmitTelemetry(regressions string, extra map[string]any)
}

type ProgressLogger struct {
	ProgressSaver             ProgressSaver
	Status                    string
	MaybePrevIsPostprocessing *bool
	MaybePrevFetchedCount     *int
	MaybePrevRemainingCount   *int
}

func NewProgressLogger(progressSaver ProgressSaver) *ProgressLogger {
	return &ProgressLogger{
		ProgressSaver:             progressSaver,
		Status:                    "",
		MaybePrevIsPostprocessing: nil,
		MaybePrevFetchedCount:     nil,
		MaybePrevRemainingCount:   nil,
	}
}

func NewMockProgressLogger(logger Logger) *ProgressLogger {
	return NewProgressLogger(NewMockProgressSaver(logger))
}

func (l *ProgressLogger) LogHtml() {
	l.Status += "h"
}

// Supposed to be called after LogHtml() but not any others
// The error can only be ErrCrawlCanceled
func (l *ProgressLogger) SaveStatus() error {
	err := l.ProgressSaver.SaveStatus(l.Status)
	if err != nil {
		return err
	}
	isPostprocessing := false
	l.trackRegressions(true, &isPostprocessing, true, nil, false, nil)
	return nil
}

// The error can only be ErrCrawlCanceled
func (l *ProgressLogger) LogAndSavePuppeteerStart() error {
	l.Status += "p"
	err := l.ProgressSaver.SaveStatus(l.Status)
	if err != nil {
		return err
	}
	isPostprocessing := false
	l.trackRegressions(true, &isPostprocessing, true, nil, false, nil)
	return nil
}

// The error can only be ErrCrawlCanceled
func (l *ProgressLogger) LogAndSavePuppeteer() error {
	l.Status += "P"
	err := l.ProgressSaver.SaveStatus(l.Status)
	if err != nil {
		return err
	}
	isPostprocessing := false
	l.trackRegressions(true, &isPostprocessing, true, nil, false, nil)
	return nil
}

// The error can only be ErrCrawlCanceled
func (l *ProgressLogger) LogAndSavePostprocessing() error {
	l.Status += "F"
	err := l.ProgressSaver.SaveStatus(l.Status)
	if err != nil {
		return err
	}
	isPostprocessing := true
	l.trackRegressions(true, &isPostprocessing, true, nil, false, nil)
	return nil
}

// The error can only be ErrCrawlCanceled
func (l *ProgressLogger) LogAndSavePostprocessingResetCount() error {
	l.Status += "F"
	err := l.ProgressSaver.SaveStatusAndCount(l.Status, nil)
	if err != nil {
		return err
	}
	isPostprocessing := true
	l.trackRegressions(true, &isPostprocessing, true, nil, true, nil)
	return nil
}

// The error can only be ErrCrawlCanceled
func (l *ProgressLogger) LogAndSavePostprocessingCounts(fetchedCount, remainingCount int) error {
	l.Status += fmt.Sprintf("F%d", remainingCount)
	err := l.ProgressSaver.SaveStatusAndCount(l.Status, &fetchedCount)
	if err != nil {
		return err
	}
	isPostprocessing := true
	l.trackRegressions(true, &isPostprocessing, true, &remainingCount, true, &fetchedCount)
	return nil
}

// The error can only be ErrCrawlCanceled
func (l *ProgressLogger) LogAndSaveFetchedCount(maybeFetchedCount *int) error {
	err := l.ProgressSaver.SaveCount(maybeFetchedCount)
	if err != nil {
		return err
	}
	l.trackRegressions(false, nil, false, nil, true, maybeFetchedCount)
	return nil
}

func (l *ProgressLogger) trackRegressions(
	trackIsPostprocessing bool, isPostprocessing *bool, trackRemainingCount bool, remainingCount *int,
	trackFetchedCount bool, fetchedCount *int,
) {
	var regressions []string
	extra := make(map[string]any)

	if trackIsPostprocessing {
		if l.MaybePrevIsPostprocessing != nil && *l.MaybePrevIsPostprocessing &&
			isPostprocessing != nil && !*isPostprocessing {

			regressions = append(regressions, "postprocessing_reset")
			extra["status"] = l.Status
		}
		l.MaybePrevIsPostprocessing = isPostprocessing
	}

	if trackRemainingCount {
		if l.MaybePrevRemainingCount != nil &&
			(remainingCount == nil || *remainingCount >= *l.MaybePrevRemainingCount) {

			regressions = append(regressions, "remaining_count_up")
			extra["prev_remaining_count"] = *l.MaybePrevRemainingCount
			if remainingCount == nil {
				extra["new_remaining_count"] = remainingCount
			} else {
				extra["new_remaining_count"] = *remainingCount
			}
		}
		l.MaybePrevRemainingCount = remainingCount
	}

	if trackFetchedCount {
		if l.MaybePrevFetchedCount != nil &&
			(fetchedCount == nil || *fetchedCount < *l.MaybePrevFetchedCount) {

			regressions = append(regressions, "fetched_count_down")
			extra["prev_fetched_count"] = *l.MaybePrevFetchedCount
			if fetchedCount == nil {
				extra["new_fetched_count"] = fetchedCount
			} else {
				extra["new_fetched_count"] = *fetchedCount
			}
		}
		l.MaybePrevFetchedCount = fetchedCount
	}

	if len(regressions) > 0 {
		l.ProgressSaver.EmitTelemetry(strings.Join(regressions, ","), extra)
	}
}

type MockProgressSaver struct {
	Logger     Logger
	Status     string
	MaybeCount *int
}

func NewMockProgressSaver(logger Logger) *MockProgressSaver {
	return &MockProgressSaver{
		Logger:     logger,
		Status:     "",
		MaybeCount: nil,
	}
}

func (s *MockProgressSaver) SaveStatusAndCount(status string, maybeCount *int) error {
	s.Logger.Info("Progress save status: %s count: %s", status, SprintIntPtr(maybeCount))
	s.Status = status
	s.MaybeCount = maybeCount
	return nil
}

func (s *MockProgressSaver) SaveStatus(status string) error {
	s.Logger.Info("Progress save status: %s", status)
	s.Status = status
	return nil
}

func (s *MockProgressSaver) SaveCount(maybeCount *int) error {
	s.Logger.Info("Progress save count: %s", SprintIntPtr(maybeCount))
	s.MaybeCount = maybeCount
	return nil
}

func (s *MockProgressSaver) EmitTelemetry(regressions string, extra map[string]any) {
	s.Logger.Info("Progress regression: %s %v", regressions, extra)
}

func SprintIntPtr(value *int) string {
	if value == nil {
		return "nil"
	} else {
		return fmt.Sprint(*value)
	}
}
