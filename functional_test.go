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
	"testing"
	"time"

	"github.com/duh-rpc/duh.go/demo"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
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

// TODO: Client example of passing `application/octet-stream` with `duh.DoBytes()`
// TODO: Update the benchmark tests

// TODO: DUH-RPC Validation Test for any endpoint
//       Not Implemented Test
//       Should error if non POST

// Is this a retryable error?
// Is this an infra error?
// Is this a failure?
// Can I tell the diff between an infra error and an error from the service?
