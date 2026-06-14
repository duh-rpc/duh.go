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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duh-rpc/duh.go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleBytesStreamsBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		duh.HandleBytes(w, r, func(r *http.Request, bw duh.BytesWriter) error {
			_, err := bw.Write([]byte("hello, bytes"))
			return err
		})
	}))
	defer server.Close()

	resp, err := http.Post(server.URL, "", nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "hello, bytes", string(body))
	assert.Equal(t, duh.ContentOctetStream, resp.Header.Get("Content-Type"))
	assert.Equal(t, duh.DUHVersion, resp.Header.Get(duh.HeaderDUHVersion))
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
}

func TestHandleBytesHandlerOverridesHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		duh.HandleBytes(w, r, func(r *http.Request, bw duh.BytesWriter) error {
			// Override the seeded Content-Type and set an app header before writing.
			bw.Header().Set("Content-Type", "text/plain; charset=utf-8")
			bw.Header().Set("X-RPC-Job-Id", "job-42")
			_, err := bw.Write([]byte("output"))
			return err
		})
	}))
	defer server.Close()

	resp, err := http.Post(server.URL, "", nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, "output", string(body))
	assert.Equal(t, "text/plain; charset=utf-8", resp.Header.Get("Content-Type"))
	assert.Equal(t, "job-42", resp.Header.Get("X-RPC-Job-Id"))
}

func TestHandleBytesErrorBeforeFirstByte(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		duh.HandleBytes(w, r, func(r *http.Request, bw duh.BytesWriter) error {
			// The handler fails before producing any output, so HandleBytes can still
			// send a standard error Reply.
			return duh.NewServiceError(duh.CodeBadRequest, "bad input", nil, nil)
		})
	}))
	defer server.Close()

	resp, err := http.Post(server.URL, "", nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, duh.DUHVersion, resp.Header.Get(duh.HeaderDUHVersion))
	assert.NotEqual(t, duh.ContentOctetStream, resp.Header.Get("Content-Type"))
	assert.Contains(t, string(body), "bad input")
}

func TestHandleBytesErrorAfterFirstByteIsCommitted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		duh.HandleBytes(w, r, func(r *http.Request, bw duh.BytesWriter) error {
			_, _ = bw.Write([]byte("partial"))
			return duh.NewServiceError(duh.CodeInternalError, "failed mid-stream", nil, nil)
		})
	}))
	defer server.Close()

	resp, err := http.Post(server.URL, "", nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Once bytes were written the 200 is committed; the error cannot become a Reply.
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, duh.ContentOctetStream, resp.Header.Get("Content-Type"))
	assert.Equal(t, "partial", string(body))
	assert.NotContains(t, string(body), "failed mid-stream")
}
