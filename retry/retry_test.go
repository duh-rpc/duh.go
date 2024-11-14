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
	"github.com/duh-rpc/duh-go"
	"github.com/duh-rpc/duh-go/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"math"
	"sync"
	"testing"
	"time"
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
	// --- Retryable errors are ---
	// 454 - Retry Request
	// 429 - Too Many Requests (Will calculate the appropriate interval to retry based on the response)
	// 500 - Internal Error (Hopefully this is a temporary error, and the server will recover)
	// 502,503,504 - Infrastructure errors which hopefully will resolve on retry.

	c.Err = errors.New("error")
	c.Attempts = 10
	var count int

	t.Run("UntilAttempts", func(t *testing.T) {
		err := retry.UntilAttempts(ctx, 2, time.Second, func(ctx context.Context, attempt int) error {
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

		_ = retry.Until(ctx, func(ctx context.Context, attempt int) error {
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
		c.Err = &testError{code: duh.CodeRetryRequest}
		c.Attempts = 5
		count = 0

		// The `retry.PolicyOnRetryable` policy will retry only on 454 retry request or until success, using
		// the default back off policy.
		_ = retry.Do(ctx, retry.PolicyOnRetryable, func(ctx context.Context, attempt int) error {
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
		customPolicy := retry.Policy{
			OnCodes: []int{duh.CodeConflict, duh.CodeTooManyRequests, 502, 503, 504},
			Interval: retry.IntervalBackOff{
				Min:    time.Millisecond,
				Max:    time.Millisecond * 100,
				Factor: 2,
			},
			Attempts: 5,
		}

		c.Err = &testError{code: duh.CodeConflict}
		c.Attempts = 10
		count = 0

		// Users can define a custom retry policy to suit their needs
		err := retry.Do(ctx, customPolicy, func(ctx context.Context, attempt int) error {
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
			Interval: retry.IntervalSleep(100 * time.Millisecond),
			// Attempts of 0 indicate infinite retries
			Attempts: 0,
		}

		c.Err = errors.New("error")
		c.Attempts = math.MaxInt
		count = 0

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			err := retry.Do(ctx, customPolicy, func(ctx context.Context, attempt int) error {
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
}

type testError struct {
	code int
}

func (t testError) ProtoMessage() proto.Message { return nil }
func (t testError) Details() map[string]string  { return nil }
func (t testError) Error() string               { return "" }
func (t testError) Message() string             { return "" }
func (t testError) Code() int {
	return t.code
}
