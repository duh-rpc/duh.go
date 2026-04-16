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

package retry_test

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	duh "github.com/duh-rpc/duh.go/v2"
	"github.com/duh-rpc/duh.go/v2/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type DoThingRequest struct{}
type DoThingResponse struct{}

type Client struct {
	Err      error
	Attempts int
}

func (c *Client) DoThing(ctx context.Context, r *DoThingRequest, resp *DoThingResponse) error {
	if c.Attempts == 0 {
		return nil
	}
	c.Attempts--
	return c.Err
}

func NewClient() *Client {
	return &Client{}
}

func TestRetry(t *testing.T) {
	c := NewClient()
	var resp DoThingResponse

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Err = errors.New("error")
	c.Attempts = 10
	var count int

	t.Run("Twice", func(t *testing.T) {
		err := retry.On(ctx, retry.Twice, func(ctx context.Context, attempt int) error {
			err := c.DoThing(ctx, &DoThingRequest{}, &resp)
			if err != nil {
				count++
				return err
			}
			return nil
		})
		require.Error(t, err)
		require.Equal(t, 2, count)
	})

	t.Run("UntilSuccess", func(t *testing.T) {
		c.Attempts = 5
		count = 0

		// The `retry.UntilSuccess` policy will retry on retryable errors until success, using the default
		// back off policy.
		_ = retry.On(ctx, retry.UntilSuccess, func(ctx context.Context, attempt int) error {
			err := c.DoThing(ctx, &DoThingRequest{}, &resp)
			if err != nil {
				count++
				return err
			}
			return nil
		})
		require.Equal(t, 5, count)
	})

	t.Run("OnRetryable", func(t *testing.T) {
		c.Err = &testError{code: "454", httpCode: duh.CodeRetryRequest}
		c.Attempts = 5
		count = 0

		// The `duh.OnRetryable` policy will retry only on retryable service codes or
		// infrastructure errors, using the default back off policy.
		_ = retry.On(ctx, duh.OnRetryable, func(ctx context.Context, attempt int) error {
			err := c.DoThing(ctx, &DoThingRequest{}, &resp)
			if err != nil {
				count++
				return err
			}
			return nil
		})
		require.Equal(t, 5, count)
	})

	t.Run("CustomPolicyBackoff", func(t *testing.T) {
		// Service codes go in OnCodes, infra codes go in OnInfraCodes
		customPolicy := retry.Policy{
			OnCodes: []int{duh.CodeConflict, duh.CodeTooManyRequests},
			Interval: retry.BackOff{
				Min:    time.Millisecond,
				Max:    time.Millisecond * 100,
				Factor: 2,
			},
			Attempts: 5,
		}

		c.Err = &testError{code: "409", httpCode: duh.CodeConflict}
		c.Attempts = 10
		count = 0

		// Users can define a custom retry policy to suit their needs
		err := retry.On(ctx, customPolicy, func(ctx context.Context, attempt int) error {
			err := c.DoThing(ctx, &DoThingRequest{}, &resp)
			if err != nil {
				count++
				return err
			}
			return nil
		})

		require.Error(t, err)
		require.Equal(t, 5, count)
	})

	t.Run("RetryUntilCancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		customPolicy := retry.Policy{
			// No Backoff, just sleep in-between retries
			Interval: retry.Sleep(100 * time.Millisecond),
			// Attempts of 0 indicate infinite retries
			Attempts: 0,
		}

		c.Err = errors.New("error")
		c.Attempts = math.MaxInt
		count = 0

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			err := retry.On(ctx, customPolicy, func(ctx context.Context, attempt int) error {
				return c.DoThing(ctx, &DoThingRequest{}, &resp)
			})
			require.Error(t, err)
			assert.Equal(t, context.Canceled, err)
			wg.Done()
		}()
		// Cancelling
		time.Sleep(2 * time.Second)
		cancel()
		wg.Wait()
	})

	t.Run("ServiceErrorInOnCodes", func(t *testing.T) {
		// Service error with code in OnCodes should be retried
		policy := retry.Policy{
			OnCodes:  []int{duh.CodeTooManyRequests},
			Interval: retry.Sleep(time.Millisecond),
			Attempts: 3,
		}

		count = 0
		err := retry.On(ctx, policy, func(ctx context.Context, attempt int) error {
			count++
			return &testError{code: "429", httpCode: duh.CodeTooManyRequests}
		})
		require.Error(t, err)
		assert.Equal(t, 3, count)
	})

	t.Run("ServiceErrorNotInOnCodes", func(t *testing.T) {
		// Service error with code NOT in OnCodes should not be retried
		policy := retry.Policy{
			OnCodes:  []int{duh.CodeTooManyRequests},
			Interval: retry.Sleep(time.Millisecond),
			Attempts: 3,
		}

		count = 0
		err := retry.On(ctx, policy, func(ctx context.Context, attempt int) error {
			count++
			return &testError{code: "400", httpCode: duh.CodeBadRequest}
		})
		require.Error(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("InfraErrorInOnInfraCodes", func(t *testing.T) {
		// Infra error with code in OnInfraCodes should be retried
		policy := retry.Policy{
			OnCodes:      []int{duh.CodeTooManyRequests},
			OnInfraCodes: []int{502, 503, 504},
			Interval:     retry.Sleep(time.Millisecond),
			Attempts:     3,
		}

		infraErr := makeInfraError(t, 503)
		count = 0
		err := retry.On(ctx, policy, func(ctx context.Context, attempt int) error {
			count++
			return infraErr
		})
		require.Error(t, err)
		assert.Equal(t, 3, count)
	})

	t.Run("InfraErrorNotInOnInfraCodes", func(t *testing.T) {
		// Infra error with code NOT in OnInfraCodes should not be retried
		policy := retry.Policy{
			OnCodes:      []int{duh.CodeTooManyRequests},
			OnInfraCodes: []int{502, 503, 504},
			Interval:     retry.Sleep(time.Millisecond),
			Attempts:     3,
		}

		infraErr := makeInfraError(t, 401)
		count = 0
		err := retry.On(ctx, policy, func(ctx context.Context, attempt int) error {
			count++
			return infraErr
		})
		require.Error(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("InfraErrorNilOnInfraCodes", func(t *testing.T) {
		// Infra error with nil OnInfraCodes should not be retried
		policy := retry.Policy{
			OnCodes:  []int{duh.CodeTooManyRequests},
			Interval: retry.Sleep(time.Millisecond),
			Attempts: 3,
		}

		infraErr := makeInfraError(t, 503)
		count = 0
		err := retry.On(ctx, policy, func(ctx context.Context, attempt int) error {
			count++
			return infraErr
		})
		require.Error(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("RateLimitResetDetail", func(t *testing.T) {
		// 429 error with ratelimit-reset detail should use that duration instead of backoff
		policy := retry.Policy{
			OnCodes: []int{duh.CodeTooManyRequests},
			Interval: retry.BackOff{
				Min:    5 * time.Second,
				Max:    10 * time.Second,
				Factor: 2,
			},
			Attempts: 2,
		}

		count = 0
		start := time.Now()
		err := retry.On(ctx, policy, func(ctx context.Context, attempt int) error {
			count++
			return &testError{
				code:     "429",
				httpCode: duh.CodeTooManyRequests,
				details:  map[string]string{"ratelimit-reset": "0.01"},
			}
		})
		elapsed := time.Since(start)
		require.Error(t, err)
		assert.Equal(t, 2, count)
		// Should have used the 10ms rate-limit duration, not the 5s backoff
		assert.Less(t, elapsed, time.Second)
	})

	t.Run("RetryAfterDetail", func(t *testing.T) {
		// 429 error with http.retry-after detail should use that duration
		policy := retry.Policy{
			OnCodes: []int{duh.CodeTooManyRequests},
			Interval: retry.BackOff{
				Min:    5 * time.Second,
				Max:    10 * time.Second,
				Factor: 2,
			},
			Attempts: 2,
		}

		count = 0
		start := time.Now()
		err := retry.On(ctx, policy, func(ctx context.Context, attempt int) error {
			count++
			return &testError{
				code:     "429",
				httpCode: duh.CodeTooManyRequests,
				details:  map[string]string{"http.retry-after": "0.01"},
			}
		})
		elapsed := time.Since(start)
		require.Error(t, err)
		assert.Equal(t, 2, count)
		// Should have used the 10ms retry-after duration, not the 5s backoff
		assert.Less(t, elapsed, time.Second)
	})

	t.Run("BackoffProgression", func(t *testing.T) {
		// Verify that backoff values actually increase across attempts (regression test for bug fix)
		backoff := retry.BackOff{
			Min:    time.Millisecond,
			Max:    time.Second,
			Factor: 2,
		}

		d1 := backoff.Next(1)
		d2 := backoff.Next(2)
		d3 := backoff.Next(3)

		// Each successive attempt should produce a longer duration
		assert.Less(t, d1, d2)
		assert.Less(t, d2, d3)

		// Verify the actual progression: Min * Factor^attempt
		assert.Equal(t, 2*time.Millisecond, d1)
		assert.Equal(t, 4*time.Millisecond, d2)
		assert.Equal(t, 8*time.Millisecond, d3)
	})

	t.Run("BackoffBugFixVerification", func(t *testing.T) {
		// Verify the On() function actually increases sleep duration across attempts.
		// The old bug passed p.Attempts (static) instead of attempt (incrementing).
		var attempts []int
		policy := retry.Policy{
			Interval: retry.Sleep(time.Millisecond),
			Attempts: 4,
		}

		err := retry.On(ctx, policy, func(ctx context.Context, attempt int) error {
			attempts = append(attempts, attempt)
			return errors.New("always fail")
		})
		require.Error(t, err)
		// Verify attempts increased: 1, 2, 3, 4
		assert.Equal(t, []int{1, 2, 3, 4}, attempts)
	})
}

// makeInfraError creates a *duh.ClientError with IsInfraError() == true by using duh.NewInfraError
// with a test HTTP response.
func makeInfraError(t *testing.T, statusCode int) error {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	resp := &http.Response{
		StatusCode: statusCode,
		Status:     strconv.Itoa(statusCode) + " " + http.StatusText(statusCode),
		Header:     http.Header{},
	}
	return duh.NewInfraError(req, resp, []byte("infra error body"))
}

type testError struct {
	details  map[string]string
	code     string
	httpCode int
}

func (t testError) ProtoMessage() proto.Message { return nil }
func (t testError) Details() map[string]string  { return t.details }
func (t testError) Error() string               { return "" }
func (t testError) Message() string             { return "" }
func (t testError) Code() string                { return t.code }
func (t testError) HTTPCode() int               { return t.httpCode }
