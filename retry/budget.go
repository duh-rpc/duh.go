package retry

import (
	"fmt"
	"math"
	"time"
)

type BudgetSlidingWindow struct {
	// The minimum number of failed operations that must occur within a moving one-minute window
	MinFailedPerMinute float64
	// The Ratio of successful operations to failed operations which must occur before
	// considering the failed operations over budget.
	Ratio float64
}

// roundDown rounds the current time down to the nearest second
func roundDown(now time.Time) time.Time {
	r := now.Round(time.Second)
	if r.After(now) {
		r = r.Add(-time.Second)
	}
	return r
}

type MovingRate struct {
	lastUpdate time.Time
	buckets    []int
	pos        int
	numBuckets int
}

func NewMovingRate(numBuckets int) *MovingRate {
	return &MovingRate{
		numBuckets: numBuckets,
	}
}

// sumBuckets sums the rates in all the buckets within the time window by weighting the oldest bucket
// within the window. By applying a fractional weight to the oldest bucket, we prevent sudden jumps in the rate
// as time progresses
func (mr *MovingRate) sumBuckets() float64 {
	if len(mr.buckets) <= mr.numBuckets {
		var sum float64
		for _, c := range mr.buckets {
			sum += float64(c)
		}
		return sum
	}

	// Add weight to the first bucket
	weight := 1.0 - float64(mr.lastUpdate.Sub(roundDown(mr.lastUpdate)))/float64(time.Second)
	sum := weight * float64(mr.buckets[0])
	first := mr.buckets[0]
	fmt.Printf("first: %d\n", first)

	// Now sum the rest of the buckets
	for i := 1; i < len(mr.buckets); i++ {
		sum += float64(mr.buckets[i])
	}

	return sum
}

// secondsInWindow returns the number of seconds we have rate information for since the last Add()
func (mr *MovingRate) secondsInWindow() float64 {
	if len(mr.buckets) <= mr.numBuckets {
		d := time.Duration(len(mr.buckets)-1) * time.Second
		d += mr.lastUpdate.Sub(roundDown(mr.lastUpdate))
		if d < time.Second {
			return 1.0
		}
		return d.Seconds()
	}

	d := time.Duration(mr.numBuckets) * time.Second
	return d.Seconds()
}

// shiftWindow manages moving the window according to the current time provided
func (mr *MovingRate) shiftWindow(now time.Time) {
	defer func() {
		mr.lastUpdate = now
	}()

	// TODO: Remove
	if mr.lastUpdate.IsZero() {
		mr.buckets = []int{0}
		return
	}

	rt := roundDown(now)
	// If current time precedes or is equal to our last update, no window
	// change is needed as time has not advanced.
	if !rt.After(mr.lastUpdate) {
		return
	}

	// Calculate the number of buckets to advance
	adv := int(rt.Sub(roundDown(mr.lastUpdate)) / time.Second)
	if adv <= 0 {
		panic(fmt.Sprintf("assert failed: adv = %d; rt = %v, mr.lastUpdate = %v", adv, rt, mr.lastUpdate))
	}

	if adv > mr.numBuckets+1 {
		adv = mr.numBuckets + 1
	}

	//for i := 0; i < adv; i++ {
	//	mr.pos = (mr.pos + i) % len(mr.buckets)
	//	mr.buckets[mr.pos] = 0
	//}

	zero := make([]int, adv)
	mr.buckets = append(mr.buckets, zero...)

	// we actually keep numBuckets+1 buckets -- the newest and oldest
	// buckets are partially evaluated so the window length stays constant.
	if del := len(mr.buckets) - (mr.numBuckets + 1); del > 0 {
		mr.buckets = mr.buckets[del:]
	}

	mr.lastUpdate = mr.lastUpdate.Add(time.Duration(adv) * time.Second)
}

func (mr *MovingRate) Add(t time.Time, n int) {
	if t.Before(mr.lastUpdate) {
		return
	}
	//fmt.Printf("Add( %s, %d )\n", t.Format(time.RFC3339Nano), n)

	mr.shiftWindow(t)
	//fmt.Printf("Add Pos: %d\n", mr.pos)
	mr.buckets[len(mr.buckets)-1] += n
	//mr.buckets[mr.pos] += n
}

func (mr *MovingRate) Rate(t time.Time) float64 {
	if t.Before(mr.lastUpdate) {
		return math.NaN()
	}

	mr.shiftWindow(t)
	//fmt.Printf("Sum: %v\n", mr.sumBuckets())
	//fmt.Printf("Seconds: %v\n", mr.secondsInWindow())
	seconds := mr.secondsInWindow()
	sum := mr.sumBuckets()
	result := sum / seconds
	return result
	//return mr.sumBuckets() / mr.secondsInWindow()

	// TODO: Collect and count the number of buckets which have hits
	// Sum all the buckets except the

}
