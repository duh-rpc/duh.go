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
	"math"
	"math/rand"
	"slices"
	"strconv"
	"time"
)

// httpCoder is satisfied by any error that carries an HTTP status code.
// duh.Error and *duh.ClientError satisfy this via HTTPCode() int.
type httpCoder interface {
	HTTPCode() int
}

// infraChecker is satisfied by errors that can report infrastructure origin.
// *duh.ClientError satisfies this via IsInfraError() bool.
type infraChecker interface {
	IsInfraError() bool
}

// detailer is satisfied by errors that carry a details map.
// duh.Error satisfies this via Details() map[string]string.
type detailer interface {
	Details() map[string]string
}

const (
	detailRateLimitReset = "ratelimit-reset"
	detailRetryAfter     = "http.retry-after"
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
	if b.Jitter > 0 {
		r := rand.Float64()
		if b.Rand != nil {
			r = b.Rand.Float64()
		}
		d = time.Duration(r * b.Jitter * float64(d))
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
	Min:    500 * time.Millisecond,
	Max:    5 * time.Second,
	Jitter: 0.2,
	Factor: 2,
}

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
	// OnCodes is a list of service response codes that trigger retry. These are checked
	// via HTTPCode() when the error is NOT an infrastructure error.
	OnCodes []int
	// OnInfraCodes is a list of infrastructure response codes that trigger retry. These are
	// checked via HTTPCode() when IsInfraError() returns true.
	// A nil value means infrastructure errors are NOT retried.
	OnInfraCodes []int
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

func shouldRetry(err error, policy Policy) bool {
	if err == nil {
		panic("err cannot be nil")
	}

	if policy.OnCodes == nil && policy.OnInfraCodes == nil {
		return true
	}

	var hc httpCoder
	if !errors.As(err, &hc) {
		return false
	}

	var ic infraChecker
	if errors.As(err, &ic) && ic.IsInfraError() {
		if policy.OnInfraCodes != nil {
			return slices.Contains(policy.OnInfraCodes, hc.HTTPCode())
		}
		return false
	}

	if policy.OnCodes != nil {
		return slices.Contains(policy.OnCodes, hc.HTTPCode())
	}
	return false
}

// rateLimitDuration extracts a rate-limit sleep duration from the error's details.
// Returns 0 if no rate-limit information is available.
func rateLimitDuration(err error) time.Duration {
	var d detailer
	if !errors.As(err, &d) {
		return 0
	}

	details := d.Details()
	if details == nil {
		return 0
	}

	for _, key := range []string{detailRateLimitReset, detailRetryAfter} {
		if v, ok := details[key]; ok {
			seconds, parseErr := strconv.ParseFloat(v, 64)
			if parseErr == nil && seconds > 0 {
				return time.Duration(seconds * float64(time.Second))
			}
		}
	}
	return 0
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
				sleepDur := rateLimitDuration(err)
				if sleepDur == 0 {
					sleepDur = p.Interval.Next(attempt)
				}
				timer := time.NewTimer(sleepDur)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
				attempt++
			} else {
				return err
			}
		}
	}
}
