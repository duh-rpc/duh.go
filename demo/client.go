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

package demo

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/duh-rpc/duh.go/v2"
	v1 "github.com/duh-rpc/duh.go/v2/proto/v1"
	json "google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Client is a simple client that calls the Service
type Client struct {
	*duh.Client
	endpoint string
}

type ClientConfig struct {
	Endpoint string
	Client   *http.Client
}

func NewClient(conf ClientConfig) *Client {
	if conf.Client == nil {
		conf.Client = &http.Client{Transport: http.DefaultTransport}
	}
	return &Client{
		Client: &duh.Client{
			Client: conf.Client,
		},
		endpoint: conf.Endpoint,
	}
}

// SayHello sends a name to the service using JSON, and the service says hello.
func (c *Client) SayHello(ctx context.Context, req *SayHelloRequest, resp *SayHelloResponse) error {
	payload, err := json.Marshal(req)
	if err != nil {
		return duh.NewClientError("while marshaling request payload: %w", err, nil)
	}

	r, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/%s", c.endpoint, "v1/say.hello"), bytes.NewReader(payload))
	if err != nil {
		return duh.NewClientError("", err, nil)
	}

	// Tell the server what kind of serialization we are sending it.
	r.Header.Set("Content-Type", duh.ContentTypeJSON)

	// Do() will handle content negotiation, error handling, and un-marshal the response
	return c.Do(r, resp)
}

// ListEvents sends a streaming request to the service which returns a stream of events.
func (c *Client) ListEvents(ctx context.Context, req *ListEventsRequest) (duh.StreamReader, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, duh.NewClientError("while marshaling request payload: %w", err, nil)
	}

	r, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/%s", c.endpoint, "v1/events.list"), bytes.NewReader(payload))
	if err != nil {
		return nil, duh.NewClientError("", err, nil)
	}

	r.Header.Set("Content-Type", duh.ContentTypeJSON)
	r.Header.Set("Accept", duh.ContentStreamJSON)
	return c.DoStream(ctx, r)
}

// DownloadBytes demonstrates DoBytes by downloading an unstructured byte stream.
func (c *Client) DownloadBytes(ctx context.Context) (io.ReadCloser, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/%s", c.endpoint, "v1/bytes.download"), nil)
	if err != nil {
		return nil, duh.NewClientError("", err, nil)
	}

	r.Header.Set("Accept", duh.ContentOctetStream)
	return c.DoBytes(ctx, r)
}

// UploadContent uploads raw content to the service at the given path.
// The contentType is sent as the request Content-Type and path is sent in the X-RPC-Path header.
func (c *Client) UploadContent(ctx context.Context, path string, contentType string, body []byte) error {
	r, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/%s", c.endpoint, "v1/content.upload"), bytes.NewReader(body))
	if err != nil {
		return duh.NewClientError("", err, nil)
	}
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("X-RPC-Path", path)
	return c.Do(r, &v1.Reply{})
}

// DownloadContent downloads content from the service at the given path.
// Returns the response headers (including Content-Type) and any error.
// The body is written into the caller-provided buf.
func (c *Client) DownloadContent(ctx context.Context, path string, buf *bytes.Buffer) (http.Header, error) {
	payload, err := json.Marshal(&ContentDownloadRequest{Path: path})
	if err != nil {
		return nil, duh.NewClientError("while marshaling request payload: %w", err, nil)
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/%s", c.endpoint, "v1/content.download"), bytes.NewReader(payload))
	if err != nil {
		return nil, duh.NewClientError("", err, nil)
	}
	r.Header.Set("Content-Type", duh.ContentTypeJSON)
	return c.DoContent(ctx, r, buf)
}

// RenderPixel sends a request to the service which calculates the pixel color of a Mandelbrot
// fractal at the given point in the image.
func (c *Client) RenderPixel(ctx context.Context, req *RenderPixelRequest, resp *RenderPixelResponse) error {
	payload, err := proto.Marshal(req)
	if err != nil {
		return duh.NewClientError("while marshaling request payload: %w", err, nil)
	}

	r, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/%s", c.endpoint, "v1/render.pixel"), bytes.NewReader(payload))
	if err != nil {
		return duh.NewClientError("", err, nil)
	}

	r.Header.Set("Content-Type", duh.ContentTypeProtoBuf)
	return c.Do(r, resp)
}
