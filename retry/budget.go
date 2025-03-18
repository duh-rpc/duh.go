package retry

import (
	"sync"
	"time"
)

// Budget is an interface that defines methods for tracking and evaluating
// the rate of failures and successes in a retry scenario.
type Budget interface {
	// IsOver returns true if the rate of failures is over budget using the time provided.
	IsOver(now time.Time) bool
	// Failure records a number of failures for the time provided.
	Failure(now time.Time, hits int)
	// Success records a number of successes for the time provided.
	Success(now time.Time, hits int)
}

type budget struct {
	mutex   sync.Mutex
	ratio   float64
	success *Rate
	failure *Rate
}

// NewBudget creates a new Budget with the specified target failure rate.
// The returned budget is thread-safe and can be used as a global budget
// for limiting the total number of retries to a resource from an application,
// regardless of concurrent threads accessing the resource.
//
// 'ratio' is the maximum ratio of failures to successes allowed within a 60 second window.
func NewBudget(ratio float64) Budget {
	return &budget{
		success: NewRate(60), // 1-minute window
		failure: NewRate(60), // 1-minute window
		ratio:   ratio,
	}
}

// Failure records a number of failures for the given time.
// This method is thread-safe.
func (b *budget) Failure(now time.Time, hits int) {
	defer b.mutex.Unlock()
	b.mutex.Lock()
	b.failure.Add(now, hits)
}

// Success records a number of successes for the given time.
// This method is thread-safe.
func (b *budget) Success(now time.Time, hits int) {
	defer b.mutex.Unlock()
	b.mutex.Lock()
	b.success.Add(now, hits)
}

// IsOver determines if the current failure rate is over the budget.
// This method is thread-safe.
func (b *budget) IsOver(now time.Time) bool {
	defer b.mutex.Unlock()
	b.mutex.Lock()

	failureRate := b.failure.Rate(now)
	successRate := b.success.Rate(now)

	// If there are no failures, we're not over budget
	if failureRate == 0 {
		return false
	}

	// We're over budget if the ratio of failures to successes exceeds the specified ratio
	return failureRate/successRate > b.ratio
}

// noOpBudget is a Budget implementation that always allows retries.
// It can be used when no budget control is desired.
type noOpBudget struct{}

// IsOver always returns false for noOpBudget, indicating that the budget is never exceeded.
func (noOpBudget) IsOver(now time.Time) bool {
	return false
}

// Failure is a no-op for noOpBudget.
func (noOpBudget) Failure(now time.Time, hits int) {}

// Success is a no-op for noOpBudget.
func (noOpBudget) Success(now time.Time, hits int) {}
