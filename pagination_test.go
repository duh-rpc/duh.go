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

package duh_test

import (
	"context"
	"errors"
	"testing"
	"time"

	duh "github.com/duh-rpc/duh.go/v2"
	"github.com/duh-rpc/duh.go/v2/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIterator(t *testing.T) {
	t.Run("HappyPath", func(t *testing.T) {
		// Iterate over 3 pages, verify all items received in order
		var cursors []string
		iter := duh.NewIterator(func(ctx context.Context, cursor string) ([]string, duh.Page, error) {
			cursors = append(cursors, cursor)
			switch cursor {
			case "":
				return []string{"a", "b"}, duh.Page{EndCursor: "cursor1", HasNextPage: true}, nil
			case "cursor1":
				return []string{"c", "d"}, duh.Page{EndCursor: "cursor2", HasNextPage: true}, nil
			case "cursor2":
				return []string{"e"}, duh.Page{EndCursor: "cursor3", HasNextPage: false}, nil
			default:
				t.Fatalf("unexpected cursor: %s", cursor)
				return nil, duh.Page{}, nil
			}
		})

		var all []string
		var page []string
		for iter.Next(context.Background(), &page) {
			all = append(all, page...)
		}

		require.NoError(t, iter.Err())
		assert.Equal(t, []string{"a", "b", "c", "d", "e"}, all)
		// Verify correct cursors were passed
		assert.Equal(t, []string{"", "cursor1", "cursor2"}, cursors)
	})

	t.Run("SinglePage", func(t *testing.T) {
		// HasNextPage == false on first page, iterator stops
		iter := duh.NewIterator(func(ctx context.Context, cursor string) ([]int, duh.Page, error) {
			return []int{1, 2, 3}, duh.Page{EndCursor: "", HasNextPage: false}, nil
		})

		var page []int
		require.True(t, iter.Next(context.Background(), &page))
		assert.Equal(t, []int{1, 2, 3}, page)

		// Second call should return false
		assert.False(t, iter.Next(context.Background(), &page))
		require.NoError(t, iter.Err())
	})

	t.Run("EmptyResult", func(t *testing.T) {
		// Fetch returns empty items with HasNextPage == false
		iter := duh.NewIterator(func(ctx context.Context, cursor string) ([]string, duh.Page, error) {
			return nil, duh.Page{HasNextPage: false}, nil
		})

		var page []string
		// Next returns true because the fetch succeeded (even with empty items)
		require.True(t, iter.Next(context.Background(), &page))
		assert.Empty(t, page)

		// Subsequent call returns false because HasNextPage was false
		assert.False(t, iter.Next(context.Background(), &page))
		require.NoError(t, iter.Err())
	})

	t.Run("FetchError", func(t *testing.T) {
		// Fetch returns error, Next returns false, Err() returns the error
		fetchErr := errors.New("connection refused")
		iter := duh.NewIterator(func(ctx context.Context, cursor string) ([]string, duh.Page, error) {
			return nil, duh.Page{}, fetchErr
		})

		var page []string
		assert.False(t, iter.Next(context.Background(), &page))
		require.ErrorIs(t, iter.Err(), fetchErr)
	})

	t.Run("RetryThenSuccess", func(t *testing.T) {
		// Fetch fails twice then succeeds, with retry policy configured
		attempts := 0
		iter := duh.NewIterator(func(ctx context.Context, cursor string) ([]string, duh.Page, error) {
			attempts++
			if attempts <= 2 {
				return nil, duh.Page{}, errors.New("transient error")
			}
			return []string{"ok"}, duh.Page{HasNextPage: false}, nil
		}, duh.WithRetryPolicy(retry.Policy{
			Interval: retry.Sleep(time.Millisecond),
			Attempts: 5,
		}))

		var page []string
		require.True(t, iter.Next(context.Background(), &page))
		assert.Equal(t, []string{"ok"}, page)
		assert.Equal(t, 3, attempts)
		require.NoError(t, iter.Err())
	})

	t.Run("RetryExhausted", func(t *testing.T) {
		// Fetch always fails, retry attempts exhausted
		iter := duh.NewIterator(func(ctx context.Context, cursor string) ([]string, duh.Page, error) {
			return nil, duh.Page{}, errors.New("persistent error")
		}, duh.WithRetryPolicy(retry.Policy{
			Interval: retry.Sleep(time.Millisecond),
			Attempts: 3,
		}))

		var page []string
		assert.False(t, iter.Next(context.Background(), &page))
		require.ErrorContains(t, iter.Err(), "persistent error")
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		// Cancel context mid-iteration, verify clean stop
		ctx, cancel := context.WithCancel(context.Background())

		callCount := 0
		iter := duh.NewIterator(func(ctx context.Context, cursor string) ([]string, duh.Page, error) {
			callCount++
			if callCount == 2 {
				cancel()
				return nil, duh.Page{}, ctx.Err()
			}
			return []string{"item"}, duh.Page{EndCursor: "next", HasNextPage: true}, nil
		})

		var page []string
		// First call succeeds
		require.True(t, iter.Next(ctx, &page))
		assert.Equal(t, []string{"item"}, page)

		// Second call gets cancelled context
		assert.False(t, iter.Next(ctx, &page))
		require.Error(t, iter.Err())
	})

	t.Run("CursorManagement", func(t *testing.T) {
		// Verify correct cursor is passed to each fetch call
		var receivedCursors []string
		iter := duh.NewIterator(func(ctx context.Context, cursor string) ([]int, duh.Page, error) {
			receivedCursors = append(receivedCursors, cursor)
			switch len(receivedCursors) {
			case 1:
				return []int{1}, duh.Page{EndCursor: "abc123", HasNextPage: true}, nil
			case 2:
				return []int{2}, duh.Page{EndCursor: "def456", HasNextPage: true}, nil
			case 3:
				return []int{3}, duh.Page{EndCursor: "ghi789", HasNextPage: false}, nil
			default:
				t.Fatal("too many calls")
				return nil, duh.Page{}, nil
			}
		})

		var page []int
		for iter.Next(context.Background(), &page) {
			// consume all pages
		}

		require.NoError(t, iter.Err())
		assert.Equal(t, []string{"", "abc123", "def456"}, receivedCursors)
	})

	t.Run("NextAfterError", func(t *testing.T) {
		// Calling Next after an error returns false without calling fetch again
		fetchCalls := 0
		iter := duh.NewIterator(func(ctx context.Context, cursor string) ([]string, duh.Page, error) {
			fetchCalls++
			return nil, duh.Page{}, errors.New("error")
		})

		var page []string
		assert.False(t, iter.Next(context.Background(), &page))
		assert.False(t, iter.Next(context.Background(), &page))
		assert.Equal(t, 1, fetchCalls)
	})
}
