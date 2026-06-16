package duh_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/duh-rpc/duh.go/v2"
	"github.com/stretchr/testify/assert"
)

// The streaming tests below each drive a handler-return path that, before
// ENG-107, leaked HandleStream's heartbeat goroutine. The leak assertion is the
// package-wide goleak check in TestMain; these tests exist to exercise the paths
// so that check has something to catch, plus a light assertion that HandleStream
// actually entered the streaming path.

// A 1ms heartbeat makes the goroutine actively tick during the handler, so a
// leaked goroutine keeps running rather than parking until the 30s default.
const leakTestHeartbeat = time.Millisecond

// streamRequest builds a recorder and a streaming request. The recorder
// implements http.Flusher, which HandleStream requires.
func streamRequest(ctx context.Context) (*httptest.ResponseRecorder, *http.Request) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/test.stream", nil).WithContext(ctx)
	r.Header.Set("Accept", duh.ContentStreamJSON)
	return w, r
}

// TestHandleStreamNilReturnNoLeak: a handler that returns nil without calling
// Close must not leave the heartbeat goroutine running (ENG-107).
func TestHandleStreamNilReturnNoLeak(t *testing.T) {
	w, r := streamRequest(context.Background())

	duh.HandleStream(w, r, func(r *http.Request, sw duh.StreamWriter) error {
		return nil
	}, &duh.StreamConfig{HeartbeatInterval: leakTestHeartbeat})

	assert.Equal(t, duh.ContentStreamJSON, w.Header().Get("Content-Type"))
}

// TestHandleStreamClientDisconnectNoLeak: the real-world trigger from ENG-101 —
// a handler that parks until the client disconnects and then returns nil (the
// demo watchStream / slip-stream firehose pattern). The heartbeat goroutine must
// still be joined.
func TestHandleStreamClientDisconnectNoLeak(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w, r := streamRequest(ctx)

	entered := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		duh.HandleStream(w, r, func(r *http.Request, sw duh.StreamWriter) error {
			close(entered)
			<-r.Context().Done()
			return nil
		}, &duh.StreamConfig{HeartbeatInterval: leakTestHeartbeat})
	}()

	<-entered
	cancel()
	<-done

	assert.Equal(t, duh.ContentStreamJSON, w.Header().Get("Content-Type"))
}

// TestHandleStreamErrorNoLeak guards the refactored error path: a handler that
// returns an error (without Close) must still join the heartbeat goroutine
// before HandleStream writes the error frame and returns.
func TestHandleStreamErrorNoLeak(t *testing.T) {
	w, r := streamRequest(context.Background())

	duh.HandleStream(w, r, func(r *http.Request, sw duh.StreamWriter) error {
		return duh.NewServiceError(duh.CodeInternalError, "boom", nil, nil)
	}, &duh.StreamConfig{HeartbeatInterval: leakTestHeartbeat})

	// The error path writes an error frame before returning.
	assert.NotEmpty(t, w.Body.Bytes())
}
