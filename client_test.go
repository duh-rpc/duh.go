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
	"github.com/duh-rpc/duh.go/v2/internal/test"
	v1 "github.com/duh-rpc/duh.go/v2/proto/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	json "google.golang.org/protobuf/encoding/protojson"
)

type badTransport struct {
}

func (t *badTransport) RoundTrip(rq *http.Request) (*http.Response, error) {
	return nil, nil
}

var badTransportClient = http.Client{Transport: &badTransport{}}

func TestClientErrors(t *testing.T) {
	service := test.NewService()
	server := httptest.NewServer(&test.Handler{Service: service})
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	for _, tt := range []struct {
		req      *test.ErrorsRequest
		details  map[string]string
		conf     test.ClientConfig
		error    string
		name     string
		msg      string
		code     string
		httpCode int
	}{
		{
			name:     "fail to marshal protobuf request",
			error:    "Client Error: while marshaling request payload: string field contains invalid UTF-8",
			conf:     test.ClientConfig{Endpoint: server.URL},
			req:      &test.ErrorsRequest{Case: string([]byte{0x80, 0x81})},
			code:     "452",
			httpCode: duh.CodeClientError,
		},
		{
			name:     "fail to create request",
			error:    "Client Error: net/http: invalid method \"invalid method\"",
			conf:     test.ClientConfig{Endpoint: ""},
			req:      &test.ErrorsRequest{Case: test.CaseInvalidMethod},
			code:     "452",
			httpCode: duh.CodeClientError,
		},
		{
			name:     "fail to create send request",
			error:    "Client Error: during client.Do(): Post \"/v1/test.errors\": unsupported protocol scheme \"\"",
			details:  map[string]string{"http.method": "POST", "http.url": "/v1/test.errors"},
			conf:     test.ClientConfig{Endpoint: ""},
			req:      &test.ErrorsRequest{},
			code:     "452",
			httpCode: duh.CodeClientError,
		},
		{
			name: "fail to create request",
			error: fmt.Sprintf("Client Error: during client.Do(): Post \"%s/v1/test.errors\": http: RoundTripper "+
				"implementation (*duh_test.badTransport) "+"returned a nil *Response with a nil error", server.URL),
			details: map[string]string{
				duh.DetailsHttpUrl:    fmt.Sprintf("%s/v1/test.errors", server.URL),
				duh.DetailsHttpMethod: "POST",
			},
			conf:     test.ClientConfig{Endpoint: server.URL, Client: &badTransportClient},
			req:      &test.ErrorsRequest{},
			code:     "452",
			httpCode: duh.CodeClientError,
		},
		{
			name:  "fail to read body of response",
			error: "Client Error: while reading response body: unexpected EOF",
			details: map[string]string{
				duh.DetailsHttpUrl:    fmt.Sprintf("%s/v1/test.errors", server.URL),
				duh.DetailsHttpMethod: "POST",
				duh.DetailsHttpStatus: "200 OK",
			},
			req:      &test.ErrorsRequest{Case: test.CaseClientIOError},
			conf:     test.ClientConfig{Endpoint: server.URL},
			code:     "452",
			httpCode: duh.CodeClientError,
		},
		{
			name: "method not implemented",
			error: fmt.Sprintf("POST %s/v1/test.errors returned 'Not Implemented' "+
				"with message: no such method; /v1/test.errors", server.URL),
			msg: "no such method; /v1/test.errors",
			details: map[string]string{
				duh.DetailsHttpUrl:    fmt.Sprintf("%s/v1/test.errors", server.URL),
				duh.DetailsCodeText:   "Not Implemented",
				duh.DetailsHttpMethod: "POST",
			},
			req:      &test.ErrorsRequest{Case: test.CaseNotImplemented},
			conf:     test.ClientConfig{Endpoint: server.URL},
			code:     "501",
			httpCode: duh.CodeNotImplemented,
		},
		{
			name: "infrastructure error",
			error: fmt.Sprintf("POST %s/v1/test.errors returned infrastructure error "+
				"404 with body: Not Found", server.URL),
			msg: "Not Found",
			details: map[string]string{
				duh.DetailsHttpUrl:    fmt.Sprintf("%s/v1/test.errors", server.URL),
				duh.DetailsHttpStatus: "404 Not Found",
				duh.DetailsHttpMethod: "POST",
			},
			req:      &test.ErrorsRequest{Case: test.CaseInfrastructureError},
			conf:     test.ClientConfig{Endpoint: server.URL},
			code:     "404",
			httpCode: http.StatusNotFound,
		},
		{
			name: "service returned an error",
			error: fmt.Sprintf("POST %s/v1/test.errors returned 'Internal Service Error' "+
				"with message: while reading the database: EOF", server.URL),
			msg: "while reading the database: EOF",
			details: map[string]string{
				duh.DetailsHttpUrl:    fmt.Sprintf("%s/v1/test.errors", server.URL),
				duh.DetailsCodeText:   "Internal Service Error",
				duh.DetailsHttpMethod: "POST",
			},
			req:      &test.ErrorsRequest{Case: test.CaseServiceReturnedError},
			conf:     test.ClientConfig{Endpoint: server.URL},
			code:     "500",
			httpCode: http.StatusInternalServerError,
		},
		{
			name: "service returned client content error",
			error: fmt.Sprintf("POST %s/v1/test.errors returned 'Client Content Error' "+
				"with message: proto:", server.URL),
			msg: "proto:",
			details: map[string]string{
				duh.DetailsHttpUrl:    fmt.Sprintf("%s/v1/test.errors", server.URL),
				duh.DetailsCodeText:   "Client Content Error",
				duh.DetailsHttpMethod: "POST",
			},
			req:      &test.ErrorsRequest{Case: test.CaseContentTypeError},
			conf:     test.ClientConfig{Endpoint: server.URL},
			code:     "455",
			httpCode: duh.CodeClientContentError,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := test.NewClient(tt.conf)
			err := c.TestErrors(ctx, tt.req)
			var e duh.Error
			require.True(t, errors.As(err, &e))
			assert.Contains(t, e.Error(), tt.error)
			assert.Contains(t, e.Message(), tt.msg)
			assert.Equal(t, tt.code, e.Code())
			assert.Equal(t, tt.httpCode, e.HTTPCode())
			for k, v := range tt.details {
				require.Contains(t, e.Details(), k)
				assert.Contains(t, e.Details()[k], v)
			}
		})
	}
}

func TestClientErrorIsInfraError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer server.Close()

	c := &duh.Client{Client: &http.Client{}}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/test", nil)
	require.NoError(t, err)

	err = c.Do(req, nil)
	require.Error(t, err)

	var ce *duh.ClientError
	require.True(t, errors.As(err, &ce))
	assert.True(t, ce.IsInfraError())
	assert.Equal(t, "502", ce.Code())
	assert.Equal(t, http.StatusBadGateway, ce.HTTPCode())
}

func TestErrorInterface(t *testing.T) {
	// Service error: Code() returns string representation of HTTP code
	err := duh.NewServiceError(duh.CodeBadRequest, "invalid input", nil, nil)
	var e duh.Error
	require.True(t, errors.As(err, &e))
	assert.Equal(t, "400", e.Code())
	assert.Equal(t, duh.CodeBadRequest, e.HTTPCode())

	// Service error with custom code
	err = duh.NewServiceErrorWithCode(duh.CodeRequestFailed, "CARD_DECLINED", "card was declined", nil, nil)
	require.True(t, errors.As(err, &e))
	assert.Equal(t, "CARD_DECLINED", e.Code())
	assert.Equal(t, duh.CodeRequestFailed, e.HTTPCode())
	assert.Equal(t, "Request Failed: card was declined", e.Error())
}

func TestServiceErrorNilErr(t *testing.T) {
	// NewServiceErrorWithCode with both msg="" and err=nil must not panic
	err := duh.NewServiceErrorWithCode(duh.CodeBadRequest, "400", "", nil, nil)
	var e duh.Error
	require.True(t, errors.As(err, &e))

	assert.Equal(t, "", e.Message())
	assert.Equal(t, "Bad Request", e.Error())
	assert.Equal(t, "400", e.Code())
	assert.Equal(t, duh.CodeBadRequest, e.HTTPCode())
}

func TestClientErrorProtoMessageNoMutation(t *testing.T) {
	// ProtoMessage() must not mutate the receiver's Message() return value
	err := duh.NewClientError("some error: %w", fmt.Errorf("inner"), nil)

	var ce *duh.ClientError
	require.True(t, errors.As(err, &ce))

	msgBefore := ce.Message()

	// Call ProtoMessage -- this previously mutated ce.msg
	reply := ce.ProtoMessage()
	require.NotNil(t, reply)

	msgAfter := ce.Message()
	assert.Equal(t, msgBefore, msgAfter)
}

func TestDoStreamErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	for _, tt := range []struct {
		name     string
		handler  http.HandlerFunc
		wantCode int
		isInfra  bool
		checkMsg string
	}{
		{
			name: "ServerReturns400WithReplyBody",
			handler: func(w http.ResponseWriter, r *http.Request) {
				reply := &v1.Reply{
					Code:    "400",
					Message: "bad request: missing field",
				}
				b, _ := json.Marshal(reply)
				w.Header().Set("Content-Type", duh.ContentTypeJSON)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write(b)
			},
			wantCode: http.StatusBadRequest,
			checkMsg: "bad request: missing field",
		},
		{
			name: "ServerReturns502WithNonReplyBody",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte("upstream timeout"))
			},
			wantCode: http.StatusBadGateway,
			isInfra:  true,
			checkMsg: "upstream timeout",
		},
		{
			name: "ServerReturns200WithWrongContentType",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("<html>oops</html>"))
			},
			wantCode: http.StatusOK,
			isInfra:  true,
			checkMsg: "<html>oops</html>",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			c := &duh.Client{Client: &http.Client{}}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v1/test.stream", nil)
			require.NoError(t, err)
			req.Header.Set("Accept", duh.ContentStreamJSON)

			sr, err := c.DoStream(ctx, req)
			require.Error(t, err)
			require.Nil(t, sr)

			var ce *duh.ClientError
			require.True(t, errors.As(err, &ce))
			assert.Equal(t, tt.wantCode, ce.HTTPCode())
			assert.Contains(t, ce.Message(), tt.checkMsg)
			if tt.isInfra {
				assert.True(t, ce.IsInfraError())
			}
		})
	}

	// Transport failure (bad URL)
	t.Run("TransportFailure", func(t *testing.T) {
		c := &duh.Client{Client: &http.Client{}}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost:1/v1/test.stream", nil)
		require.NoError(t, err)
		req.Header.Set("Accept", duh.ContentStreamJSON)

		sr, err := c.DoStream(ctx, req)
		require.Error(t, err)
		require.Nil(t, sr)

		var ce *duh.ClientError
		require.True(t, errors.As(err, &ce))
		assert.Equal(t, duh.CodeClientError, ce.HTTPCode())
	})
}

func TestDoBytesErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	for _, tt := range []struct {
		name     string
		handler  http.HandlerFunc
		wantCode int
		isInfra  bool
		checkMsg string
	}{
		{
			name: "ServerReturns400WithReplyBody",
			handler: func(w http.ResponseWriter, r *http.Request) {
				reply := &v1.Reply{
					Code:    "400",
					Message: "bad request: invalid params",
				}
				b, _ := json.Marshal(reply)
				w.Header().Set("Content-Type", duh.ContentTypeJSON)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write(b)
			},
			wantCode: http.StatusBadRequest,
			checkMsg: "bad request: invalid params",
		},
		{
			name: "ServerReturns200WithWrongContentType",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("not binary data"))
			},
			wantCode: http.StatusOK,
			isInfra:  true,
			checkMsg: "not binary data",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			c := &duh.Client{Client: &http.Client{}}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/bytes.download", nil)
			require.NoError(t, err)
			req.Header.Set("Accept", duh.ContentOctetStream)

			rc, err := c.DoBytes(ctx, req)
			require.Error(t, err)
			require.Nil(t, rc)

			var ce *duh.ClientError
			require.True(t, errors.As(err, &ce))
			assert.Equal(t, tt.wantCode, ce.HTTPCode())
			assert.Contains(t, ce.Message(), tt.checkMsg)
			if tt.isInfra {
				assert.True(t, ce.IsInfraError())
			}
		})
	}

	// Success case: returns body as io.ReadCloser
	t.Run("SuccessReturnsBody", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", duh.ContentOctetStream)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello, bytes"))
		}))
		defer server.Close()

		c := &duh.Client{Client: &http.Client{}}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/bytes.download", nil)
		require.NoError(t, err)
		req.Header.Set("Accept", duh.ContentOctetStream)

		rc, err := c.DoBytes(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, rc)

		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		assert.Equal(t, "hello, bytes", string(data))
		require.NoError(t, rc.Close())
	})

	// Transport failure
	t.Run("TransportFailure", func(t *testing.T) {
		c := &duh.Client{Client: &http.Client{}}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:1/v1/bytes.download", nil)
		require.NoError(t, err)

		rc, err := c.DoBytes(ctx, req)
		require.Error(t, err)
		require.Nil(t, rc)

		var ce *duh.ClientError
		require.True(t, errors.As(err, &ce))
		assert.Equal(t, duh.CodeClientError, ce.HTTPCode())
	})
}
