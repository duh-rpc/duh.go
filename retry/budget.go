package retry

type BudgetSlidingWindow struct {
	// The minimum number of failed operations that must occur within a moving one-minute window
	MinFailedPerMinute float64
	// The Ratio of successful operations to failed operations which must occur before
	// considering the failed operations over budget.
	Ratio float64
}

// TODO: Implement a budget using `Rate` of successful and unsuccessful requests
