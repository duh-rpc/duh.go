/*
Copyright 2023 Derrick J Wippler

Licensed under the MIT License, you may obtain a copy of the License at

https://opensource.org/license/mit/ or in the root of this code repo

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package retry

import (
	"context"
	"errors"
	"github.com/duh-rpc/duh.go"
	"math"
	"math/rand"
	"net/http"
	"slices"
	"time"
)

type Interval interface {
	Next(attempts int) time.Duration
}

type BackOff struct {
	Min    time.Duration
	Max    time.Duration
	Factor float64
	Jitter float64
	Rand   *rand.Rand
}

func (b BackOff) Next(attempts int) time.Duration {
	d := time.Duration(float64(b.Min) * math.Pow(b.Factor, float64(attempts)))
	if b.Rand != nil {
		d = time.Duration(b.Rand.Float64() * b.Jitter * float64(d))
	}
	if d > b.Max {
		return b.Max
	}
	if d < b.Min {
		return b.Min
	}
	return d
}

var DefaultBackOff = BackOff{
	Rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	Min:    500 * time.Millisecond,
	Max:    5 * time.Second,
	Jitter: 0.2,
	Factor: 2,
}

// RetryableCodes is a list of duh return codes which are retryable.
var RetryableCodes = []int{duh.CodeRetryRequest, duh.CodeTooManyRequests, duh.CodeInternalError,
	http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout}

type Sleep time.Duration

func (s Sleep) Next(_ int) time.Duration {
	return time.Duration(s)
}

type Policy struct {
	// Interval is an interface which dictates how long the retry should sleep between attempts. Retry comes with
	// two implementations called retry.BackOff which implements a backoff and retry.Sleep which is a static sleep
	// value with no backoff.
	//
	// 	backoffPolicy := retry.Policy{
	//		Interval: retry.BackOff{
	//			Min:    time.Millisecond,
	//			Max:    time.Millisecond * 100,
	//			Factor: 2,
	//		},
	//		Attempts: 5,
	//	}
	//
	// 	sleepPolicy := retry.Policy{
	//		Interval: retry.Sleep(5 * time.Seconds),
	//		Attempts: 5,
	//	}
	//
	Interval Interval // BackOff or Sleep
	// OnCodes is a list of codes which will cause a retry. If an error occurs which is not an implementation
	// of duh.Error and OnCodes then a retry will NOT occur.
	OnCodes []int
	// Attempts is the number of "attempts" before retry returns an error to the caller.
	// Attempts includes the first attempt, it is a count of the number of "total attempts" that
	// will be attempted.
	Attempts int // 0 for infinite
}

// Twice policy will retry 'twice' if there was an error. Uses the default back off policy
var Twice = Policy{
	Interval: DefaultBackOff,
	Attempts: 2,
}

var UntilSuccess = Policy{
	Interval: DefaultBackOff,
	Attempts: 0,
}

// OnRetryable is intended to be used by clients interacting with a duh rpc service. It will retry
// indefinitely as long as the service returns a retryable error. Users who which to cancel the indefinite retry
// should cancel the context.
var OnRetryable = Policy{
	Interval: DefaultBackOff,
	OnCodes:  RetryableCodes,
	Attempts: 0,
}

func shouldRetry(err error, policy Policy) bool {
	if err == nil {
		panic("err cannot be nil")
	}

	if policy.OnCodes != nil {
		var duhErr duh.Error
		if errors.As(err, &duhErr) {
			return slices.Contains(policy.OnCodes, duhErr.Code())
		}
	} else {
		return true
	}
	return false
}

func On(ctx context.Context, p Policy, operation func(context.Context, int) error) error {
	attempt := 1
	if p.Interval == nil {
		panic("Policy.Interval cannot be nil")
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err := operation(ctx, attempt)
			if err == nil || (p.Attempts != 0 && attempt >= p.Attempts) {
				return err
			}

			if shouldRetry(err, p) {
				time.Sleep(p.Interval.Next(p.Attempts))
				attempt++
			} else {
				return err
			}
		}
	}
}
