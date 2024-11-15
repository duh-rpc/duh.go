package retry

import (
	"fmt"
	"math"
	"time"
)

type MovingRateRing struct {
	last       time.Time // The last time
	buckets    []int
	numBuckets int
	pos        int
}

func NewMovingRateRing(numBuckets int) *MovingRateRing {
	return &MovingRateRing{
		buckets:    make([]int, numBuckets+1),
		numBuckets: numBuckets, // TODO: Remove
	}
}

// shiftWindow manages moving the window according to the time provided.
// Although `numBuckets` is the window size, we keep one additional bucket
// `numBuckets+1` so we can preform a weighted average using the oldest bucket
// just outside our window.
func (mr *MovingRateRing) shiftWindow(now time.Time) {
	defer func() {
		mr.last = now
	}()

	rt := roundDown(now)
	// If this is our first time, or the current time precedes or is equal to our
	// last update, no window change is needed as time has not advanced.
	if mr.last.IsZero() || !rt.After(mr.last) {
		return
	}

	// Calculate the number of buckets to advance
	adv := int(rt.Sub(roundDown(mr.last)) / time.Second)
	if adv <= 0 {
		panic(fmt.Sprintf("assert failed: adv = %d; rt = %v, mr.last = %v", adv, rt, mr.last))
	}

	if adv > mr.numBuckets+1 {
		adv = mr.numBuckets + 1
	}

	// advance through the buckets starting at head and
	// clear any hits for each bucket we advance.
	pos := mr.pos
	for i := 0; i < adv; i++ {
		pos = (pos + 1) % len(mr.buckets)
		mr.buckets[pos] = 0
	}
	mr.pos = (mr.pos + adv) % len(mr.buckets)
	mr.last = mr.last.Add(time.Duration(adv) * time.Second)
}

func (mr *MovingRateRing) Add(now time.Time, hits int) {
	if now.Before(mr.last) {
		return
	}

	mr.shiftWindow(now)
	mr.buckets[mr.pos] += hits
}

func (mr *MovingRateRing) Rate(now time.Time) float64 {
	if now.Before(mr.last) {
		return math.NaN()
	}

	mr.shiftWindow(now)

	var first, sum float64
	var bucketsUsed int
	pos := mr.pos

	for i := 0; i < len(mr.buckets); i++ {
		pos = (pos + 1) % len(mr.buckets)
		if mr.buckets[pos] == 0 {
			continue
		}
		bucketsUsed++

		if first == 0 {
			first = float64(mr.buckets[pos])
			continue
		}
		sum += float64(mr.buckets[pos])
	}
	var seconds time.Duration

	// Avoid adding weight to a window that isn't full
	if bucketsUsed < len(mr.buckets) {
		seconds = time.Duration(bucketsUsed-1) * time.Second
		seconds += mr.last.Sub(roundDown(mr.last))
		sum += first
	} else {
		seconds = time.Duration(mr.numBuckets) * time.Second
		weight := 1.0 - float64(mr.last.Sub(roundDown(mr.last)))/float64(time.Second)
		sum += weight * first
	}

	if seconds < time.Second {
		seconds = time.Second
	}

	result := sum / seconds.Seconds()
	return result
}
