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
	"strconv"
	"strings"
	"time"

	v1 "github.com/duh-rpc/duh.go/v2/proto/v1"
	"github.com/duh-rpc/duh.go/v2/stream"
	"golang.org/x/net/http2"
	json "google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Client struct {
	Client *http.Client
	// MaxFramePayload is the maximum payload size for streaming frames.
	// 0 uses DefaultMaxFramePayload.
	MaxFramePayload int
}

// DefaultMaxFramePayload is the default maximum payload size for streaming frames.
const DefaultMaxFramePayload = 4 * MegaByte

const (
	DetailsHttpCode       = "http.code"
	DetailsHttpUrl        = "http.url"
	DetailsHttpMethod     = "http.method"
	DetailsHttpStatus     = "http.status"
	DetailsHttpBody       = "http.body"
	DetailsCodeText       = "duh.code-text"
	DetailsHttpRetryAfter = "http.retry-after"
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
			code:     strconv.Itoa(CodeClientError),
			httpCode: CodeClientError,
		}
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
		return NewClientError(
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
		return NewClientError(
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

	// Extract Retry-After header on 429 responses
	if resp.StatusCode == CodeTooManyRequests {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			details[DetailsHttpRetryAfter] = ra
		}
	}

	return &ClientError{
		code:     reply.Code,
		httpCode: resp.StatusCode,
		msg:      reply.Message,
		details:  details,
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
		code:         strconv.Itoa(resp.StatusCode),
		httpCode:     resp.StatusCode,
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
		code:     strconv.Itoa(CodeClientError),
		httpCode: CodeClientError,
		details:  details,
		err:      err,
	}
}

// DoStream sends the request and returns a StreamReader that reads structured
// stream frames from the response body. The caller must call StreamReader.Close
// when done to release resources.
func (c *Client) DoStream(ctx context.Context, req *http.Request) (StreamReader, error) {
	childCtx, cancel := context.WithCancel(ctx)
	req = req.WithContext(childCtx)

	resp, err := c.Client.Do(req)
	if err != nil {
		cancel()
		return nil, NewClientError("during client.Do(): %w", err, map[string]string{
			DetailsHttpUrl:    req.URL.String(),
			DetailsHttpMethod: req.Method,
		})
	}

	if resp.StatusCode != CodeOK {
		cancel()
		return nil, handleErrorResponse(req, resp)
	}

	ct := TrimSuffix(resp.Header.Get("Content-Type"), ";,")
	ct = strings.TrimSpace(strings.ToLower(ct))

	var unmarshalFn func([]byte, proto.Message) error
	switch ct {
	case ContentStreamJSON:
		unmarshalFn = json.Unmarshal
	case ContentStreamProtoBuf:
		unmarshalFn = proto.Unmarshal
	default:
		cancel()
		var body bytes.Buffer
		_, _ = io.Copy(&body, resp.Body)
		_ = resp.Body.Close()
		return nil, NewInfraError(req, resp, body.Bytes())
	}

	maxPayload := c.MaxFramePayload
	if maxPayload == 0 {
		maxPayload = DefaultMaxFramePayload
	}

	return &streamReader{
		r:         stream.NewReader(resp.Body, maxPayload),
		resp:      resp,
		unmarshal: unmarshalFn,
		cancel:    cancel,
	}, nil
}

// DoBytes sends the request and returns the response body as an io.ReadCloser
// for unstructured (octet-stream) responses. The caller must close the returned
// ReadCloser when done.
func (c *Client) DoBytes(ctx context.Context, req *http.Request) (io.ReadCloser, error) {
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, NewClientError("during client.Do(): %w", err, map[string]string{
			DetailsHttpUrl:    req.URL.String(),
			DetailsHttpMethod: req.Method,
		})
	}

	if resp.StatusCode != CodeOK {
		return nil, handleErrorResponse(req, resp)
	}

	ct := TrimSuffix(resp.Header.Get("Content-Type"), ";,")
	ct = strings.TrimSpace(strings.ToLower(ct))

	if ct != ContentOctetStream {
		var body bytes.Buffer
		_, _ = io.Copy(&body, resp.Body)
		_ = resp.Body.Close()
		return nil, NewInfraError(req, resp, body.Bytes())
	}

	return resp.Body, nil
}

// handleErrorResponse reads the response body and classifies the error based
// on the content type and body contents. It closes the response body.
func handleErrorResponse(req *http.Request, resp *http.Response) error {
	defer func() { _ = resp.Body.Close() }()

	var body bytes.Buffer
	if _, err := io.Copy(&body, resp.Body); err != nil {
		return &ClientError{
			err: fmt.Errorf("while reading response body: %w", err),
			details: map[string]string{
				DetailsHttpUrl:    req.URL.String(),
				DetailsHttpMethod: req.Method,
				DetailsHttpStatus: resp.Status,
			},
			code:     strconv.Itoa(CodeClientError),
			httpCode: CodeClientError,
		}
	}

	ct := TrimSuffix(resp.Header.Get("Content-Type"), ";,")
	switch strings.TrimSpace(strings.ToLower(ct)) {
	case ContentTypeJSON:
		var reply v1.Reply
		if err := json.Unmarshal(body.Bytes(), &reply); err != nil {
			return NewInfraError(req, resp, body.Bytes())
		}
		return NewReplyError(req, resp, &reply)
	case ContentTypeProtoBuf:
		var reply v1.Reply
		if err := proto.Unmarshal(body.Bytes(), &reply); err != nil {
			return NewInfraError(req, resp, body.Bytes())
		}
		return NewReplyError(req, resp, &reply)
	default:
		return NewInfraError(req, resp, body.Bytes())
	}
}
