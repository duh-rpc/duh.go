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
	"github.com/duh-rpc/duh-go"
	"math"
	"math/rand"
	"net/http"
	"slices"
	"time"
)

type Budget interface {
	Allow(context.Context) error
}

type Interval interface {
	Next(attempts int) time.Duration
}

// IntervalBackOff implements backoff algorithm with jitter
//
//	interval := retry.IntervalBackOff{
//			Rand: rand.New(rand.NewSource(time.Now().UnixNano())),
//			Min:    500 * time.Millisecond,
//			Max:    5 * time.Second,
//			Jitter: 0.2,
//			Factor: 0.5,
//		},
type IntervalBackOff struct {
	Min    time.Duration
	Max    time.Duration
	Factor float64
	Jitter float64
	Rand   *rand.Rand
}

// TODO: Include an example backoff retry interval chart in the documentation, similar too
// https://cloud.google.com/java/docs/reference/google-http-client/1.43.0/com.google.api.client.util.ExponentialBackOff
//

func (b IntervalBackOff) Next(attempts int) time.Duration {
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

// IntervalSleep is a constant sleep interval which sleeps for the duration provided before retrying.
type IntervalSleep time.Duration

func (s IntervalSleep) Next(_ int) time.Duration {
	return time.Duration(s)
}

// Policy is the policy retry uses to decide how often how many times an operation should be retried
//
//  policy = retry.Policy{
//  Interval: retry.IntervalBackOff{
//		Rand: rand.New(rand.NewSource(time.Now().UnixNano())),
//		// These values taken from Google Java Client
//		Min:    500 * time.Millisecond,
//		Max:    5 * time.Second,
//		Jitter: 0.2,
//		Factor: 0.5,
//	},
//	Budget:   nil,
//	Attempts: 0,
//}

type Policy struct {
	// Interval is an interface which dictates how long the retry should sleep between attempts. Retry comes with
	// two implementations called retry.IntervalBackOff which implements a backoff and retry.IntervalSleep which
	// is a constant sleep interval with no backoff.
	Interval Interval

	// OnCodes is a list of codes which will cause a retry. If an error occurs which is not an implementation
	// of duh.Error and OnCodes then a retry will NOT occur.
	OnCodes []int

	// Budget is the budget used to determine if a retry should proceed. Budgets block
	// retries until requests are under budget or the provided context is cancelled.
	// Using a budget avoids generating excess load on the resource being retried,
	// once it has recovered. It also improves recovery time once the resource
	// has recovered. Set to `nil` to ignore budgets
	// See https://medium.com/yandex/good-retry-bad-retry-an-incident-story-648072d3cee6
	Budget Budget

	// Attempts is the number of "attempts" before an individual retry returns an error to the caller.
	// Attempts includes the first attempt, it is a ount of the number of "total attempts" that
	// will be attempted.
	Attempts int // 0 for infinite
}

// PolicyDefault is the policy shared by package level Until(), and UntilAttempts() functions
var PolicyDefault = Policy{
	Interval: IntervalBackOff{
		Rand: rand.New(rand.NewSource(time.Now().UnixNano())),
		// These values taken from Google Java Client
		Min:    500 * time.Millisecond,
		Max:    5 * time.Second,
		Jitter: 0.2,
		Factor: 0.5,
	},
	Budget:   nil,
	Attempts: 0, // Infinite retries
}

// PolicyOnRetryable is intended to be used by clients interacting with a duh rpc service. It will retry
// indefinitely as long as the service returns a retryable error. Users who wish to cancel the indefinite retry
// should cancel the context.
var PolicyOnRetryable = Policy{
	Interval: PolicyDefault.Interval,
	OnCodes:  RetryableCodes,
	Budget:   nil,
	Attempts: 0,
}

// Until retries the provided operation using exponential backoff and the default budget until the
// context is cancelled
func Until(ctx context.Context, op func(context.Context, int) error) error {
	return Do(ctx, PolicyDefault, op)
}

// UntilAttempts retries the provided operation using exponential backoff and the default budget until the
// number of attempts has been reached or context is cancelled
func UntilAttempts(ctx context.Context, attempts int, sleep time.Duration, op func(context.Context, int) error) error {
	return Do(ctx, Policy{
		Interval: IntervalBackOff{
			Rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
			Max:    sleep * 10,
			Min:    sleep,
			Jitter: 0.2,
			Factor: 0.5,
		},
		Budget:   PolicyDefault.Budget,
		Attempts: attempts,
	}, op)
}

func Do(ctx context.Context, p Policy, op func(context.Context, int) error) error {
	attempt := 1
	if p.Interval == nil {
		panic("Policy.Interval cannot be nil")
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err := op(ctx, attempt)
			if err == nil || (p.Attempts != 0 && attempt >= p.Attempts) {
				return err
			}

			if shouldRetry(ctx, p, err) {
				time.Sleep(p.Interval.Next(p.Attempts))
				attempt++
			} else {
				return err
			}
		}
	}
}

func shouldRetry(ctx context.Context, policy Policy, err error) bool {
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

	if policy.Budget != nil {
		// Allow blocks until the context is cancelled or the budget allows
		// the retry to occur.
		if policy.Budget.Allow(ctx) == nil {
			return true
		}
	}
	return false
}

// RetryableCodes is a list of duh return codes which are retryable.
var RetryableCodes = []int{duh.CodeRetryRequest, duh.CodeTooManyRequests, duh.CodeInternalError,
	http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout}
