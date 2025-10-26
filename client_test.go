package duh_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/duh-rpc/duh.go"
	"github.com/duh-rpc/duh.go/internal/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
		req     *test.ErrorsRequest
		details map[string]string
		conf    test.ClientConfig
		error   string
		name    string
		msg     string
		code    int
	}{
		{
			name:  "fail to marshal protobuf request",
			error: "Client Error: while marshaling request payload: string field contains invalid UTF-8",
			conf:  test.ClientConfig{Endpoint: server.URL},
			req: &test.ErrorsRequest{
				Case: string([]byte{0x80, 0x81}),
			},
			code: duh.CodeClientError,
		},
		{
			name:  "fail to create request",
			error: "Client Error: net/http: invalid method \"invalid method\"",
			conf:  test.ClientConfig{Endpoint: ""},
			req:   &test.ErrorsRequest{Case: test.CaseInvalidMethod},
			code:  duh.CodeClientError,
		},
		{
			name:    "fail to create send request",
			error:   "Client Error: during client.Do(): Post \"/v1/test.errors\": unsupported protocol scheme \"\"",
			details: map[string]string{"http.method": "POST", "http.url": "/v1/test.errors"},
			conf:    test.ClientConfig{Endpoint: ""},
			req:     &test.ErrorsRequest{},
			code:    duh.CodeClientError,
		},
		{
			name: "fail to create request",
			error: fmt.Sprintf("Client Error: during client.Do(): Post \"%s/v1/test.errors\": http: RoundTripper "+
				"implementation (*duh_test.badTransport) "+"returned a nil *Response with a nil error", server.URL),
			details: map[string]string{
				duh.DetailsHttpUrl:    fmt.Sprintf("%s/v1/test.errors", server.URL),
				duh.DetailsHttpMethod: "POST",
			},
			conf: test.ClientConfig{Endpoint: server.URL, Client: &badTransportClient},
			req:  &test.ErrorsRequest{},
			code: duh.CodeClientError,
		},
		{
			name:  "fail to read body of response",
			error: "Transport Error: while reading response body: unexpected EOF",
			details: map[string]string{
				duh.DetailsHttpUrl:    fmt.Sprintf("%s/v1/test.errors", server.URL),
				duh.DetailsHttpMethod: "POST",
				duh.DetailsHttpStatus: "200 OK",
			},
			req:  &test.ErrorsRequest{Case: test.CaseClientIOError},
			conf: test.ClientConfig{Endpoint: server.URL},
			code: duh.CodeTransportError,
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
			req:  &test.ErrorsRequest{Case: test.CaseNotImplemented},
			conf: test.ClientConfig{Endpoint: server.URL},
			code: duh.CodeNotImplemented,
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
			req:  &test.ErrorsRequest{Case: test.CaseInfrastructureError},
			conf: test.ClientConfig{Endpoint: server.URL},
			code: http.StatusNotFound,
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
			req:  &test.ErrorsRequest{Case: test.CaseServiceReturnedError},
			conf: test.ClientConfig{Endpoint: server.URL},
			code: http.StatusInternalServerError,
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
			req:  &test.ErrorsRequest{Case: test.CaseContentTypeError},
			conf: test.ClientConfig{Endpoint: server.URL},
			code: duh.CodeClientContentError,
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
			for k, v := range tt.details {
				require.Contains(t, e.Details(), k)
				assert.Contains(t, e.Details()[k], v)
			}
		})
	}
}
