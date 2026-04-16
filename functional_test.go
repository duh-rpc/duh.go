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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/duh-rpc/duh.go/v2"
	"github.com/duh-rpc/duh.go/v2/demo"
	"github.com/duh-rpc/duh.go/v2/internal/test"
	"github.com/duh-rpc/duh.go/v2/stream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	json "google.golang.org/protobuf/encoding/protojson"
)

func TestDemoHappyPath(t *testing.T) {
	// Create a new instance of our service
	service := demo.NewService()

	// Create a new server which handles the HTTP requests for our service
	server := httptest.NewServer(&demo.Handler{Service: service})
	defer server.Close()

	// Create a new client to make RPC calls to the service via the HTTP Handler
	c := demo.NewClient(demo.ClientConfig{Endpoint: server.URL})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	// Test happy path JSON request and response
	{
		req := demo.SayHelloRequest{
			Name: "Admiral Thrawn",
		}
		var resp demo.SayHelloResponse
		assert.NoError(t, c.SayHello(ctx, &req, &resp))
		assert.Equal(t, "Hello, Admiral Thrawn", resp.Message)
	}

	// Test happy path Protobuf request and response
	{
		req := demo.RenderPixelRequest{
			Complexity: 1024,
			Height:     2048,
			Width:      2048,
			I:          1,
			J:          1,
		}

		var resp demo.RenderPixelResponse
		assert.NoError(t, c.RenderPixel(ctx, &req, &resp))
		assert.Equal(t, int64(72), resp.Gray)
	}
}

func TestStreamingHappyPath(t *testing.T) {
	service := test.NewService()
	server := httptest.NewServer(&test.Handler{Service: service})
	defer server.Close()

	c := test.NewClient(test.ClientConfig{Endpoint: server.URL})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	sr, err := c.TestStream(ctx, &test.StreamRequest{Count: 5})
	require.NoError(t, err)

	// Collect all items
	var items []*test.StreamItem
	for {
		var item test.StreamItem
		err := sr.Recv(&item)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		items = append(items, &item)
	}

	require.Len(t, items, 5)
	for i, item := range items {
		assert.Equal(t, int64(i), item.Sequence)
		assert.Equal(t, fmt.Sprintf("item-%d", i), item.Data)
	}

	// Verify subsequent Recv returns EOF
	var extra test.StreamItem
	assert.Equal(t, io.EOF, sr.Recv(&extra))

	// Close is safe to call after EOF
	require.NoError(t, sr.Close())
}

func TestStreamingErrorFrame(t *testing.T) {
	service := test.NewService()
	server := httptest.NewServer(&test.Handler{Service: service})
	defer server.Close()

	c := test.NewClient(test.ClientConfig{Endpoint: server.URL})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	sr, err := c.TestStream(ctx, &test.StreamRequest{
		Count:   3,
		ErrorAt: "database connection lost",
	})
	require.NoError(t, err)

	// Receive 3 data frames successfully
	for i := 0; i < 3; i++ {
		var item test.StreamItem
		require.NoError(t, sr.Recv(&item))
		assert.Equal(t, int64(i), item.Sequence)
	}

	// Next Recv returns the error frame
	var item test.StreamItem
	err = sr.Recv(&item)
	require.Error(t, err)

	var duhErr duh.Error
	require.True(t, errors.As(err, &duhErr))
	assert.Equal(t, "500", duhErr.Code())
	assert.Contains(t, duhErr.Message(), "database connection lost")

	require.NoError(t, sr.Close())
}

func TestStreamingCloseWithPayload(t *testing.T) {
	service := test.NewService()
	server := httptest.NewServer(&test.Handler{Service: service})
	defer server.Close()

	c := test.NewClient(test.ClientConfig{Endpoint: server.URL})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	sr, err := c.TestStream(ctx, &test.StreamRequest{
		Count:            2,
		CloseWithPayload: true,
	})
	require.NoError(t, err)

	// Receive 2 data frames
	for i := 0; i < 2; i++ {
		var item test.StreamItem
		require.NoError(t, sr.Recv(&item))
		assert.Equal(t, int64(i), item.Sequence)
	}

	// Receive the final frame payload (3rd Recv returns nil error with the payload)
	var finalItem test.StreamItem
	require.NoError(t, sr.Recv(&finalItem))
	assert.Equal(t, int64(2), finalItem.Sequence)

	// Next Recv returns EOF
	var extra test.StreamItem
	assert.Equal(t, io.EOF, sr.Recv(&extra))

	require.NoError(t, sr.Close())
}

func TestStreamingBadAcceptHeader(t *testing.T) {
	service := test.NewService()
	server := httptest.NewServer(&test.Handler{Service: service})
	defer server.Close()

	c := test.NewClient(test.ClientConfig{Endpoint: server.URL})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	// Manually construct a request with a non-stream Accept header
	payload, err := json.Marshal(&test.StreamRequest{Count: 1})
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		server.URL+"/v1/test.stream", io.NopCloser(
			io.NewSectionReader(
				readerAtFromBytes(payload), 0, int64(len(payload)),
			),
		))
	require.NoError(t, err)
	req.Header.Set("Content-Type", duh.ContentTypeJSON)
	req.Header.Set("Accept", duh.ContentTypeJSON)

	// DoStream should return an error because the server returns a 400 Reply
	sr, err := c.Client.DoStream(ctx, req)
	require.Error(t, err)
	require.Nil(t, sr)

	var duhErr *duh.ClientError
	require.True(t, errors.As(err, &duhErr))
	assert.Equal(t, http.StatusBadRequest, duhErr.HTTPCode())
	assert.Contains(t, duhErr.Message(), "Accept header")
}

// readerAtFromBytes creates a bytes.Reader that implements io.ReaderAt.
func readerAtFromBytes(b []byte) *bytesReaderAt {
	return &bytesReaderAt{data: b}
}

type bytesReaderAt struct {
	data []byte
}

func (r *bytesReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n = copy(p, r.data[off:])
	if n < len(p) {
		err = io.EOF
	}
	return
}

func TestStreamingSendAfterClose(t *testing.T) {
	// Channel to capture the Send-after-Close error from the handler
	sendErr := make(chan error, 1)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		duh.HandleStream(w, r, func(r *http.Request, sw duh.StreamWriter) error {
			// Close the stream immediately
			if err := sw.Close(nil); err != nil {
				sendErr <- fmt.Errorf("close failed: %w", err)
				return err
			}

			// Attempt to Send after Close -- should fail
			sendErr <- sw.Send(&test.StreamItem{Sequence: 1, Data: "after-close"})
			return nil
		})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	c := &duh.Client{Client: &http.Client{}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		server.URL+"/v1/test.stream", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", duh.ContentStreamJSON)

	sr, err := c.DoStream(ctx, req)
	require.NoError(t, err)

	// Client receives EOF because the stream was closed with nil payload
	var item test.StreamItem
	assert.Equal(t, io.EOF, sr.Recv(&item))
	require.NoError(t, sr.Close())

	// Verify the server-side Send after Close returned an error
	serverErr := <-sendErr
	require.Error(t, serverErr)
	assert.Contains(t, serverErr.Error(), "stream is closed")
}

func TestStreamingServerDisconnect(t *testing.T) {
	// Raw handler that writes one data frame but doesn't write a final frame
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", duh.ContentStreamJSON)

		sw := stream.NewWriter(w)
		payload, _ := json.Marshal(&test.StreamItem{Sequence: 0, Data: "only-item"})
		_ = sw.WriteFrame(stream.FlagData, payload)

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Return without writing a final frame -- connection closes
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	c := &duh.Client{Client: &http.Client{}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		server.URL+"/v1/test.stream", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", duh.ContentStreamJSON)

	sr, err := c.DoStream(ctx, req)
	require.NoError(t, err)

	// First Recv succeeds
	var item test.StreamItem
	require.NoError(t, sr.Recv(&item))
	assert.Equal(t, int64(0), item.Sequence)
	assert.Equal(t, "only-item", item.Data)

	// Second Recv returns io.ErrUnexpectedEOF because no final frame was sent
	err = sr.Recv(&item)
	assert.Equal(t, io.ErrUnexpectedEOF, err)

	require.NoError(t, sr.Close())
}

func TestStreamingContextCancel(t *testing.T) {
	// Handler that sends frames with a delay between each
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		duh.HandleStream(w, r, func(r *http.Request, sw duh.StreamWriter) error {
			for i := 0; i < 100; i++ {
				if err := sw.Send(&test.StreamItem{
					Sequence: int64(i),
					Data:     fmt.Sprintf("item-%d", i),
				}); err != nil {
					return err
				}
				select {
				case <-sw.Context().Done():
					return sw.Context().Err()
				case <-time.After(50 * time.Millisecond):
				}
			}
			return sw.Close(nil)
		})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	c := &duh.Client{Client: &http.Client{}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		server.URL+"/v1/test.stream", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", duh.ContentStreamJSON)

	sr, err := c.DoStream(ctx, req)
	require.NoError(t, err)

	// Read 2 frames
	for i := 0; i < 2; i++ {
		var item test.StreamItem
		require.NoError(t, sr.Recv(&item))
		assert.Equal(t, int64(i), item.Sequence)
	}

	// Cancel the context by closing the stream reader (which cancels the child context)
	require.NoError(t, sr.Close())

	// Subsequent Recv should return EOF (since we closed)
	var item test.StreamItem
	assert.Equal(t, io.EOF, sr.Recv(&item))
}

func TestDoBytesHappyPath(t *testing.T) {
	service := demo.NewService()
	server := httptest.NewServer(&demo.Handler{Service: service})
	defer server.Close()

	c := demo.NewClient(demo.ClientConfig{Endpoint: server.URL})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	rc, err := c.DownloadBytes(ctx)
	require.NoError(t, err)
	require.NotNil(t, rc)

	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "hello, bytes", string(data))
	require.NoError(t, rc.Close())
}

// TODO: Update the benchmark tests

// TODO: DUH-RPC Validation Test for any endpoint
//       Not Implemented Test
//       Should error if non POST

// Is this a retryable error?
// Is this an infra error?
// Is this a failure?
// Can I tell the diff between an infra error and an error from the service?
