package retry_test

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/duh-rpc/duh-go/retry"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type Point struct {
	Time    time.Time
	Success int
	Failed  int
}

func TestBudgetGraph(t *testing.T) {

	client := http.Client{
		Transport: &http.Transport{
			ForceAttemptHTTP2:     true,
			MaxIdleConnsPerHost:   100_000,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	//report(t, retry.Policy{
	//	Interval: retry.IntervalBackOff{
	//		Rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	//		Min:    time.Millisecond,
	//		Max:    500 * time.Millisecond,
	//		Factor: 1.01,
	//		Jitter: 0.50,
	//	},
	//	Budget:   nil,
	//	Attempts: 0,
	//}, client, "no-budget")

	report(t, retry.Policy{
		Interval: retry.IntervalBackOff{
			Rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
			Min:    time.Millisecond,
			Max:    500 * time.Millisecond,
			Factor: 1.01,
			Jitter: 0.50,
		},
		// TODO: Implement Budget
		Budget:   nil,
		Attempts: 0,
	}, client, "with-budget")
}

func report(t *testing.T, policy retry.Policy, client http.Client, prefix string) {
	var hits []Point
	var upTime []Point
	var mutex sync.Mutex
	var down atomic.Bool

	// TODO: Remove
	prefix = fmt.Sprintf("/Users/thrawn/Development/marimo/%s", prefix)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			mutex.Lock()
			hits = append(hits, Point{Time: time.Now(), Failed: 1})
			mutex.Unlock()

			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("NOT OK"))
			return
		}
		mutex.Lock()
		hits = append(hits, Point{Time: time.Now(), Success: 1})
		mutex.Unlock()
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(10)))
		//time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Wait until we are at the nearest second before starting the test to
	// ensure round up/down doesn't skew results in un-expected ways
	now := time.Now()
	start := roundUp(now, time.Second)
	fmt.Printf("Run Time: %+v\n", now)
	time.Sleep(start.Sub(now))

	start = time.Now()
	fmt.Printf("Start Time: %+v\n", start)
	start = start.Round(time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				_ = retry.Do(ctx, policy, func(ctx context.Context, i int) error {
					return request(&client, server.URL)
				})
				// Wait to be cancelled or for our next request. This simulates
				// a client who is doing work in between making requests to the server.
				select {
				case <-ctx.Done():
					return
				//case <-time.After(time.Duration(rand.Intn(100)) * time.Millisecond):
				default: // <-- to simulate no time between requests
				}
			}
		}()
	}

	upTime = append(upTime, Point{Time: time.Now().Round(time.Millisecond * 250), Success: 1500})
	time.Sleep(2 * time.Second)
	upTime = append(upTime, Point{Time: time.Now().Round(time.Millisecond * 250), Success: 1500})
	upTime = append(upTime, Point{Time: time.Now().Round(time.Millisecond * 250), Success: 0})
	down.Store(true)
	time.Sleep(4 * time.Second)
	upTime = append(upTime, Point{Time: time.Now().Round(time.Millisecond * 250), Success: 0})
	upTime = append(upTime, Point{Time: time.Now().Round(time.Millisecond * 250), Success: 1500})
	time.Sleep(100 * time.Millisecond)
	down.Store(false)
	time.Sleep(1900 * time.Millisecond)
	upTime = append(upTime, Point{Time: time.Now().Round(time.Millisecond * 250), Success: 1500})

	// Cancel the context and wait for the go routines to end
	cancel()
	wg.Wait()
	stop := time.Now()

	r := rollup(hits)
	writeRollup(t, r, start, fmt.Sprintf("%s-data.csv", prefix))
	writeUpTime(t, upTime, start, fmt.Sprintf("%s-uptime.csv", prefix))
	writeInterval(t, start, stop.Add(250*time.Millisecond), 250*time.Millisecond,
		fmt.Sprintf("%s-intervals.csv", prefix))
}

func writeInterval(t *testing.T, start time.Time, stop time.Time, interval time.Duration, name string) {
	f, err := os.Create(name)
	if err != nil {
		panic(err)
	}
	w := csv.NewWriter(f)
	_ = w.Write([]string{"time"})
	start = start.Round(interval)
	for i := 0; ; i++ {
		n := start.Add(interval * time.Duration(i))
		if n.After(stop) {
			break
		}
		_ = w.Write([]string{fmt.Sprintf("%.1f", n.Sub(start).Seconds())})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		panic(err)
	}
	_ = f.Close()
	t.Logf("Wrote: %s", name)
}

func writeUpTime(t *testing.T, upTime []Point, now time.Time, name string) {
	f, err := os.Create(name)
	if err != nil {
		panic(err)
	}
	w := csv.NewWriter(f)
	_ = w.Write([]string{"time", "up"})

	for _, point := range upTime {
		ts := point.Time.Sub(now).Seconds()
		_ = w.Write([]string{fmt.Sprintf("%.1f", ts), fmt.Sprintf("%d", point.Success)})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		panic(err)
	}
	_ = f.Close()
	t.Logf("Wrote: %s", name)
}

func writeRollup(t *testing.T, rollup []Point, now time.Time, name string) {
	f, err := os.Create(name)
	if err != nil {
		panic(err)
	}
	w := csv.NewWriter(f)
	_ = w.Write([]string{"time", "success", "failed"})

	for _, point := range rollup {
		ts := point.Time.Sub(now).Seconds()
		_ = w.Write([]string{
			fmt.Sprintf("%.1f", ts),
			fmt.Sprintf("%d", point.Success),
			fmt.Sprintf("%d", point.Failed),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		panic(err)
	}
	_ = f.Close()
	t.Logf("Wrote: %s", name)
}

func request(client *http.Client, url string) error {
	resp, err := client.Get(url)
	if err != nil {
		panic(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read the response body to allow connection reuse
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return errors.New("request failed")
	}
	return nil
}

func rollup(series []Point) []Point {
	buckets := make(map[time.Time]*Point)
	for _, p := range series {
		key := roundUp(p.Time, 100*time.Millisecond)
		if o, ok := buckets[key]; ok {
			o.Failed += p.Failed
			o.Success += p.Success
		} else {
			p.Time = key
			buckets[key] = &Point{Time: key, Success: p.Success, Failed: p.Failed}
		}
	}
	var result []Point
	for k, v := range buckets {
		result = append(result, Point{Time: k, Success: v.Success, Failed: v.Failed})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Time.Before(result[j].Time)
	})
	return result
}

// roundUp rounds the current time up
func roundUp(now time.Time, interval time.Duration) time.Time {
	r := now.Round(interval)
	if r.Before(now) {
		r = r.Add(interval)
	}
	return r
}
