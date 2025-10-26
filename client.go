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

package duh

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	v1 "github.com/duh-rpc/duh.go/proto/v1"
	"golang.org/x/net/http2"
	json "google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Client struct {
	Client *http.Client
}

const (
	DetailsHttpCode   = "http.code"
	DetailsHttpUrl    = "http.url"
	DetailsHttpMethod = "http.method"
	DetailsHttpStatus = "http.status"
	DetailsHttpBody   = "http.body"
	DetailsCodeText   = "duh.code-text"
)

var (
	// DefaultClient is the default HTTP client to use when making RPC calls.
	// We use the HTTP/1 client as it outperforms both GRPC and HTTP/2
	// See:
	// * https://github.com/duh-rpc/duh.go-benchmarks
	// * https://github.com/golang/go/issues/47840
	// * https://www.emcfarlane.com/blog/2023-05-15-grpc-servehttp
	// * https://github.com/kgersen/h3ctx
	DefaultClient = HTTP1Client

	// HTTP1Client is the default golang http with a limit on Idle connections
	HTTP1Client = &Client{
		Client: &http.Client{
			Transport: &http.Transport{
				IdleConnTimeout:     90 * time.Second,
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				MaxConnsPerHost:     0,
			},
		},
	}

	// HTTP2Client is a client configured for H2C HTTP/2
	HTTP2Client = &Client{
		Client: &http.Client{
			Transport: &http2.Transport{
				// So http2.Transport doesn't complain the URL scheme isn't 'https'
				AllowHTTP: true,
				// Pretend we are dialing a TLS endpoint. (Note, we ignore the passed tls.Config)
				DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, network, addr)
				},
			},
		},
	}
)

// TODO: Move this documentation to retry.UntilSuccess
// DoWithRetry is identical to Do() except it will retry using the default retry.UntilSuccess which
// will retry all requests that return one of the following status codes.
//
//	500 - Internal Error
//	502 - Bad Gateway
//	503 - Service Unavailable
//	505 - Gateway Timeout
//	429 - Too Many Requests
//
// On 429 if the server provides a reset-time, DoWithRetry will calculate the appropriate retry time and
// sleep until that time occurs or until the context is canceled.

// DoOctetStream sends the request and expects a `application/octet-stream` response from the server.
// If server doesn't respond with `application/octet-stream` then it is assumed to be an error of v1.Reply
// If the reply isn't a v1.Reply then the body of the response is returned as an error.
// TODO: Implement this and clean this up
func (c *Client) DoOctetStream(req *http.Request, code *int, r io.ReadCloser) error {
	return nil
}

// Do calls http.Client.Do() and un-marshals the response into the proto struct passed.
// In the case of unexpected request or response errors, Do will return *duh.ClientError
// with as much detail as possible.
func (c *Client) Do(req *http.Request, out proto.Message) error {
	// Preform the HTTP call
	resp, err := c.Client.Do(req)
	if err != nil {
		return NewClientError("during client.Do(): %w", err, map[string]string{
			DetailsHttpUrl:    req.URL.String(),
			DetailsHttpMethod: req.Method,
		})
	}
	defer func() { _ = resp.Body.Close() }()

	var body bytes.Buffer
	// Copy the response into a buffer
	if _, err = io.Copy(&body, resp.Body); err != nil {
		return &ClientError{
			err: fmt.Errorf("while reading response body: %w", err),
			details: map[string]string{
				DetailsHttpUrl:    req.URL.String(),
				DetailsHttpMethod: req.Method,
				DetailsHttpStatus: resp.Status,
			},
			code: CodeTransportError,
		}
	}

	// If we get a code that is not a known DUH code, then don't attempt to un-marshal,
	// instead read the body and return an error
	if !IsDUHCode(resp.StatusCode) {
		return NewInfraError(req, resp, body.Bytes())
	}

	// Handle content negotiation and un-marshal the response
	mt := TrimSuffix(resp.Header.Get("Content-Type"), ";,")
	switch strings.TrimSpace(strings.ToLower(mt)) {
	case ContentTypeJSON:
		return c.handleJSONResponse(req, resp, body.Bytes(), out)
	case ContentTypeProtoBuf:
		return c.handleProtobufResponse(req, resp, body.Bytes(), out)
	default:
		return NewInfraError(req, resp, body.Bytes())
	}
}

func (c *Client) handleJSONResponse(req *http.Request, resp *http.Response, body []byte, out proto.Message) error {
	if resp.StatusCode != CodeOK {
		var reply v1.Reply
		if err := json.Unmarshal(body, &reply); err != nil {
			// Assume the body is not a Reply structure because
			// the server is not respecting the spec.
			return NewInfraError(req, resp, body)
		}
		return NewReplyError(req, resp, &reply)
	}

	if err := json.Unmarshal(body, out); err != nil {
		return NewServiceError(CodeClientError,
			"", fmt.Errorf("while parsing response body '%s': %w", body, err), nil)
	}
	return nil
}

func (c *Client) handleProtobufResponse(req *http.Request, resp *http.Response, body []byte, out proto.Message) error {
	if resp.StatusCode != CodeOK {
		var reply v1.Reply
		if err := proto.Unmarshal(body, &reply); err != nil {
			return NewInfraError(req, resp, body)
		}
		return NewReplyError(req, resp, &reply)
	}

	if err := proto.Unmarshal(body, out); err != nil {
		return NewServiceError(CodeClientError,
			"", fmt.Errorf("while parsing response body '%s': %w", body, err), nil)
	}
	return nil
}

// NewReplyError returns an error that originates from the service implementation, and does not originate from
// the client or infrastructure.
//
// This method is intended to be used by client implementations to pass v1.Reply responses
// back to the caller as an error.
func NewReplyError(req *http.Request, resp *http.Response, reply *v1.Reply) error {
	details := map[string]string{
		DetailsHttpCode:   fmt.Sprintf("%d", resp.StatusCode),
		DetailsCodeText:   CodeText(resp.StatusCode),
		DetailsHttpUrl:    req.URL.String(),
		DetailsHttpMethod: req.Method,
	}

	for k, v := range reply.Details {
		details[k] = v
	}

	// TODO: Handle CodeTooManyRequests, include a way to easily get those retry values
	//  so retry.On() can get them.
	return &ClientError{
		code:    int(reply.Code),
		msg:     reply.Message,
		details: details,
	}
}

// NewInfraError returns an error that originates from the infrastructure, and does not originate from
// the client or service implementation.
func NewInfraError(req *http.Request, resp *http.Response, body []byte) error {
	return &ClientError{
		details: map[string]string{
			DetailsHttpCode:   fmt.Sprintf("%d", resp.StatusCode),
			DetailsHttpBody:   string(body),
			DetailsHttpUrl:    req.URL.String(),
			DetailsHttpStatus: resp.Status,
			DetailsHttpMethod: req.Method,
		},
		msg:          string(body),
		code:         resp.StatusCode,
		isInfraError: true,
	}
}

// NewClientError returns an error that originates with the client code not from the service
// implementation or from the infrastructure.
func NewClientError(msg string, err error, details map[string]string) error {
	if msg != "" {
		if err != nil {
			err = fmt.Errorf(msg, err)
		} else {
			err = errors.New(msg)
		}
	}
	return &ClientError{
		code:    CodeClientError,
		details: details,
		err:     err,
	}
}
