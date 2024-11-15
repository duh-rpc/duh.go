package retry

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

//func TestGRPCMovingRateMinute(t *testing.T) {
//	now := time.Date(2024, 1, 1, 1, 1, 1, 1, time.Local)
//	t.Run("Second", func(t *testing.T) {
//		rate := NewMovingRate()
//		for i := 0; i < 60; i++ {
//			rate.Add(now.Add(time.Duration(i)*time.Second), 3)
//		}
//		r := rate.Rate(now.Add(60 * time.Second))
//		assert.Equal(t, "3.00", fmt.Sprintf("%.2f", r))
//		fmt.Printf("Current rate: %.2f hits per second\n", r)
//	})
//
//	t.Run("SubSecond", func(t *testing.T) {
//		rate := NewMovingRate()
//		for i := 0; i < 60; i++ {
//			rate.Add(now.Add(time.Duration(i)*(10*time.Millisecond)), 3)
//		}
//		r := rate.Rate(now.Add(60 * time.Second))
//		assert.Equal(t, "3.00", fmt.Sprintf("%.2f", r))
//		fmt.Printf("Current rate: %.2f hits per second\n", r)
//	})
//}

func TestOriginalMovingRate(t *testing.T) {
	cases := []struct {
		calls  []int
		expect string
	}{
		{
			calls:  []int{5},
			expect: "25.00",
		},
		{
			calls:  []int{5, 3},
			expect: "6.67",
		},
		{
			calls:  []int{5, 5, 1},
			expect: "5.00",
		},
		{
			calls:  []int{5, 5, 5, 5, 5, 5, 5, 5, 5, 1},
			expect: "5.00",
		},
		{
			calls: []int{
				5, // partial value
				5, 5, 5, 5, 5, 5, 5, 5, 5, 1},
			expect: "5.00",
		},
		{
			calls: []int{
				1000000, // partial value
				2, 2, 2, 2, 2, 2, 2, 2, 2, 2,
			},
			expect: "80002.00",
		},
		{
			calls: []int{
				2, 2, 2, 2, // old
				5, // partial
				1, 1, 1, 1, 0, 0, 0, 0, 0, 1,
			},
			expect: "0.90",
		},
	}

	for _, c := range cases {
		mr := &movingRate{
			BucketLength: time.Second,
			BucketNum:    10,
		}
		//t.Logf("BEFORE mr = %+v", mr)

		tm := time.Date(2018, time.February, 22, 22, 24, 53, 200000000, time.UTC)
		for _, n := range c.calls {
			tm = tm.Add(mr.BucketLength)
			for j := 0; j < n; j++ {
				mr.Add(tm, 1)
			}
		}

		assert.Equal(t, c.expect, fmt.Sprintf("%.2f", mr.Rate(tm)))

		t.Logf("AFTER  mr = %+v", mr)
	}
}

func TestModifiedMovingRate(t *testing.T) {
	t.Run("TestCases", func(t *testing.T) {

		cases := []struct {
			calls  []int
			expect string
		}{
			//{
			//	calls:  []int{5},
			//	expect: "5.00",
			//},
			//{
			//	calls:  []int{5, 3},
			//	expect: "6.67",
			//},
			//{
			//	calls:  []int{5, 5, 1},
			//	expect: "5.00",
			//},
			//{
			//	calls:  []int{5, 5, 5, 5, 5, 5, 5, 5, 5, 1},
			//	expect: "5.00",
			//},
			//{
			//	calls: []int{
			//		5, // partial value
			//		5, 5, 5, 5, 5, 5, 5, 5, 5, 1},
			//	expect: "5.00",
			//},
			//{
			//	calls: []int{
			//		1000000, // partial value
			//		2, 2, 2, 2, 2, 2, 2, 2, 2, 2,
			//	},
			//	expect: "80002.00",
			//},
			{
				calls: []int{
					2, 2, 2, 2, // old
					5, // partial
					1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
				},
				expect: "1.40",
			},
		}

		for _, c := range cases {
			mr := NewMovingRate(10)

			tm := time.Date(2018, time.February, 22, 22, 24, 53, 200000000, time.UTC)
			fmt.Printf("Start Time: %s\n", tm.String())
			for _, n := range c.calls {
				tm = tm.Add(time.Second)
				for j := 0; j < n; j++ {
					mr.Add(tm, 1)
				}
			}

			assert.Equal(t, c.expect, fmt.Sprintf("%.2f", mr.Rate(tm)))

			t.Logf("AFTER  mr = %+v", mr)
		}
	})

	t.Run("TimeGap", func(t *testing.T) {
		mr := NewMovingRate(10)
		now := time.Date(2018, time.February, 22, 22, 24, 53, 200000000, time.UTC)
		mr.Add(now, 5)
		assert.Equal(t, "5.00", fmt.Sprintf("%.2f", mr.Rate(now)))

		now = now.Add(time.Minute)
		mr.Add(now, 5)
		assert.Equal(t, "0.50", fmt.Sprintf("%.2f", mr.Rate(now)))

		now = now.Add(time.Minute)
		assert.Equal(t, "0.00", fmt.Sprintf("%.2f", mr.Rate(now)))

		fmt.Printf("No more allocs\n")
		now = now.Add(time.Minute)
		mr.Add(now, 5)
		assert.Equal(t, "0.50", fmt.Sprintf("%.2f", mr.Rate(now)))
		t.Logf("AFTER  mr = %+v", mr)
	})
}

func TestRing(t *testing.T) {
	ring := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	pos := 3
	var first, sum float64
	var count int

	for i := 0; i < len(ring); i++ {
		pos = (pos + 1) % len(ring)
		fmt.Printf("ring[%d] = %d\n", pos, ring[pos])
		if ring[pos] == 0 {
			continue
		}
		count++

		if first == 0 {
			first = float64(ring[pos])
			continue
		}
		sum += float64(ring[pos])
	}

	if count == len(ring) {
		sum += first * 0.8
		fmt.Printf("weighted sum = %.2f\n", sum)
	} else {
		sum += first
		fmt.Printf("not full sum = %.2fd\n", sum)
	}
}

func TestMovingRateRing(t *testing.T) {
	t.Run("TestCases", func(t *testing.T) {

		cases := []struct {
			name   string
			calls  []int
			expect string
		}{
			{
				name:   "one-bucket",
				calls:  []int{5},
				expect: "5.00",
			},
			{
				name:   "two-bucket",
				calls:  []int{5, 3},
				expect: "6.67",
			},
			{
				name:   "three-bucket",
				calls:  []int{5, 5, 1},
				expect: "5.00",
			},
			{
				name:   "ten-bucket",
				calls:  []int{5, 5, 5, 5, 5, 5, 5, 5, 5, 1},
				expect: "5.00",
			},
			{
				name: "weighted-avg",
				calls: []int{
					5, // outside the window
					5, 5, 5, 5, 5, 5, 5, 5, 5, 1},
				expect: "5.00",
			},
			{
				name: "weighted-avg-large",
				calls: []int{
					1000000, // outside the window
					2, 2, 2, 2, 2, 2, 2, 2, 2, 2,
				},
				expect: "80002.00",
			},
			{
				name: "shift-window",
				calls: []int{
					2, 2, 2, 2, // removed by window shift
					5, // outside the window
					1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
				},
				expect: "1.40",
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				mr := NewMovingRateRing(10)
				tm := time.Date(2018, time.February, 22, 22, 24, 53, 200000000, time.UTC)
				fmt.Printf("Start Time: %s\n", tm.String())
				for _, n := range c.calls {
					tm = tm.Add(time.Second)
					for j := 0; j < n; j++ {
						mr.Add(tm, 1)
					}
				}
				assert.Equal(t, c.expect, fmt.Sprintf("%.2f", mr.Rate(tm)))
				t.Logf("AFTER  mr = %+v", mr)
			})
		}
	})

	t.Run("TimeGap", func(t *testing.T) {
		mr := NewMovingRate(10)
		now := time.Date(2018, time.February, 22, 22, 24, 53, 200000000, time.UTC)
		mr.Add(now, 5)
		assert.Equal(t, "5.00", fmt.Sprintf("%.2f", mr.Rate(now)))

		now = now.Add(time.Minute)
		mr.Add(now, 5)
		assert.Equal(t, "0.50", fmt.Sprintf("%.2f", mr.Rate(now)))

		now = now.Add(time.Minute)
		assert.Equal(t, "0.00", fmt.Sprintf("%.2f", mr.Rate(now)))

		fmt.Printf("No more allocs\n")
		now = now.Add(time.Minute)
		mr.Add(now, 5)
		assert.Equal(t, "0.50", fmt.Sprintf("%.2f", mr.Rate(now)))
		t.Logf("AFTER  mr = %+v", mr)
	})
}

func BenchmarkMovingRate(b *testing.B) {
	m := NewMovingRate(60)
	now := time.Date(2018, time.February, 22, 22, 24, 53, 200000000, time.UTC)

	b.Run("Moving Rate", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			now = now.Add(time.Second)
			m.Add(now, 5)
		}
		m.Rate(now.Add(time.Second))
		b.ReportAllocs()
	})
}

func BenchmarkOldMovingRate(b *testing.B) {
	m := &movingRate{
		BucketLength: time.Second,
		BucketNum:    60,
	}
	now := time.Date(2018, time.February, 22, 22, 24, 53, 200000000, time.UTC)

	b.Run("Moving Rate", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			now = now.Add(time.Second)
			m.Add(now, 5)
		}
		m.Rate(now.Add(time.Second))
		b.ReportAllocs()
	})
}
