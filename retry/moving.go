package retry

import (
	"fmt"
	"math"
	"time"
)

func timeRoundDown(t time.Time, d time.Duration) time.Time {
	rt := t.Round(d)
	if rt.After(t) {
		rt = rt.Add(-d)
	}

	return rt
}

type movingRate struct {
	BucketLength time.Duration
	BucketNum    int

	counts     []int
	lastUpdate time.Time
}

func newMovingRate() *movingRate {
	return &movingRate{
		BucketLength: time.Second,
		BucketNum:    60,
	}
}

func (mr *movingRate) count() float64 {
	// history is not yet fully initialized
	if len(mr.counts) <= mr.BucketNum {
		var s float64
		for _, c := range mr.counts {
			s += float64(c)
		}
		return s
	}

	oldestFraction := 1.0 -
		float64(mr.lastUpdate.Sub(timeRoundDown(mr.lastUpdate, mr.BucketLength)))/
			float64(mr.BucketLength)

	s := oldestFraction * float64(mr.counts[0])
	for i := 1; i < len(mr.counts); i++ {
		s += float64(mr.counts[i])
	}

	return s
}

func (mr *movingRate) second() float64 {
	if len(mr.counts) == 0 {
		return 0.0
	}

	// history is not yet fully initialized
	if len(mr.counts) <= mr.BucketNum {
		d := time.Duration(len(mr.counts)-1) * mr.BucketLength
		d += mr.lastUpdate.Sub(timeRoundDown(mr.lastUpdate, mr.BucketLength))
		return d.Seconds()
	}

	d := time.Duration(mr.BucketNum) * mr.BucketLength
	return d.Seconds()
}

func (mr *movingRate) shift(n int) {
	if n > mr.BucketNum+1 {
		n = mr.BucketNum + 1
	}

	zero := make([]int, n)
	mr.counts = append(mr.counts, zero...)

	// we actually keep numBuckets+1 buckets -- the newest and oldest
	// buckets are partially evaluated so the window length stays constant.
	if del := len(mr.counts) - (mr.BucketNum + 1); del > 0 {
		mr.counts = mr.counts[del:]
	}

	mr.lastUpdate = timeRoundDown(mr.lastUpdate, mr.BucketLength).Add(time.Duration(n) * mr.BucketLength)

}

func (mr *movingRate) forward(t time.Time) {
	defer func() {
		mr.lastUpdate = t
	}()

	if mr.lastUpdate.IsZero() {
		mr.counts = []int{0}
		return
	}

	rt := timeRoundDown(t, mr.BucketLength)
	if !rt.After(mr.lastUpdate) {
		return
	}

	n := int(rt.Sub(timeRoundDown(mr.lastUpdate, mr.BucketLength)) / mr.BucketLength)
	if n <= 0 {
		panic(fmt.Sprintf("assertion failure: n = %d, want >0; rt = %v, mr.lastUpdate = %v, mr.BucketLength = %v",
			n, rt, mr.lastUpdate, mr.BucketLength))
	}

	mr.shift(n)
}

func (mr *movingRate) Add(t time.Time, n int) {
	if t.Before(mr.lastUpdate) {
		return
	}
	//fmt.Printf("Add( %s, %d )\n", t.Format(time.RFC3339Nano), n)

	mr.forward(t)
	mr.counts[len(mr.counts)-1] += n
}

func (mr *movingRate) Rate(t time.Time) float64 {
	if t.Before(mr.lastUpdate) {
		return math.NaN()
	}

	mr.forward(t)
	return mr.count() / mr.second()
}
