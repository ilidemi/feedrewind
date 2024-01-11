package crawler

import (
	"fmt"
	"strings"
)

type ProgressSaver interface {
	SaveStatusAndCount(status string, maybeCount *int)
	SaveStatus(status string)
	SaveCount(maybeCount *int)
	EmitTelemetry(regressions string, extra map[string]any)
}

type ProgressLogger struct {
	ProgressSaver             ProgressSaver
	Status                    string
	MaybePrevIsPostprocessing *bool
	MaybePrevFetchedCount     *int
	MaybePrevRemainingCount   *int
}

func NewProgressLogger(progressSaver ProgressSaver) ProgressLogger {
	return ProgressLogger{
		ProgressSaver:             progressSaver,
		Status:                    "",
		MaybePrevIsPostprocessing: nil,
		MaybePrevFetchedCount:     nil,
		MaybePrevRemainingCount:   nil,
	}
}

func NewMockProgressLogger(logger Logger) ProgressLogger {
	return NewProgressLogger(NewMockProgressSaver(logger))
}

func (l *ProgressLogger) LogHtml() {
	l.Status += "h"
}

// Supposed to be called after LogHtml() but not any others
func (l *ProgressLogger) SaveStatus() {
	l.ProgressSaver.SaveStatus(l.Status)
	isPostprocessing := false
	l.trackRegressions(true, &isPostprocessing, true, nil, false, nil)
}

func (l *ProgressLogger) LogAndSavePuppeteerStart() {
	l.Status += "p"
	l.ProgressSaver.SaveStatus(l.Status)
	isPostprocessing := false
	l.trackRegressions(true, &isPostprocessing, true, nil, false, nil)
}

func (l *ProgressLogger) LogAndSavePuppeteer() {
	l.Status += "P"
	l.ProgressSaver.SaveStatus(l.Status)
	isPostprocessing := false
	l.trackRegressions(true, &isPostprocessing, true, nil, false, nil)
}

func (l *ProgressLogger) LogAndSavePostprocessing() {
	l.Status += "F"
	l.ProgressSaver.SaveStatus(l.Status)
	isPostprocessing := true
	l.trackRegressions(true, &isPostprocessing, true, nil, false, nil)
}

func (l *ProgressLogger) LogAndSavePostprocessingResetCount() {
	l.Status += "F"
	l.ProgressSaver.SaveStatusAndCount(l.Status, nil)
	isPostprocessing := true
	l.trackRegressions(true, &isPostprocessing, true, nil, true, nil)
}

func (l *ProgressLogger) LogAndSavePostprocessingCounts(fetchedCount, remainingCount int) {
	l.Status += fmt.Sprintf("F%d", remainingCount)
	l.ProgressSaver.SaveStatusAndCount(l.Status, &fetchedCount)
	isPostprocessing := true
	l.trackRegressions(true, &isPostprocessing, true, &remainingCount, true, &fetchedCount)
}

func (l *ProgressLogger) LogAndSaveFetchedCount(maybeFetchedCount *int) {
	l.ProgressSaver.SaveCount(maybeFetchedCount)
	l.trackRegressions(false, nil, false, nil, true, maybeFetchedCount)
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

func (s *MockProgressSaver) SaveStatusAndCount(status string, maybeCount *int) {
	s.Logger.Info("Progress save status: %s count: %s", status, SprintIntPtr(maybeCount))
	s.Status = status
	s.MaybeCount = maybeCount
}

func (s *MockProgressSaver) SaveStatus(status string) {
	s.Logger.Info("Progress save status: %s", status)
	s.Status = status
}

func (s *MockProgressSaver) SaveCount(maybeCount *int) {
	s.Logger.Info("Progress save count: %s", SprintIntPtr(maybeCount))
	s.MaybeCount = maybeCount
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
