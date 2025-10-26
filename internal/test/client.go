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

package test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/duh-rpc/duh.go"
	"google.golang.org/protobuf/proto"
	"net/http"
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

// TestErrors is used in test suite to test error handling
func (c *Client) TestErrors(ctx context.Context, req *ErrorsRequest) error {
	m := http.MethodPost
	if req.Case == CaseInvalidMethod {
		m = "invalid method"
	}

	var payload []byte
	var err error

	if req.Case == CaseContentTypeError {
		// Send bogus content
		payload = []byte("This is not a protobuf message")
	} else {
		payload, err = proto.Marshal(req)
		if err != nil {
			return duh.NewClientError("while marshaling request payload: %w", err, nil)
		}
	}

	r, err := http.NewRequestWithContext(ctx, m,
		fmt.Sprintf("%s/%s", c.endpoint, "v1/test.errors"), bytes.NewReader(payload))
	if err != nil {
		return duh.NewClientError("", err, nil)
	}

	r.Header.Set("Content-Type", duh.ContentTypeProtoBuf)
	var resp proto.Message
	return c.Do(r, resp)
}
