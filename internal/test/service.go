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
	"context"
	"fmt"

	"github.com/duh-rpc/duh.go/v2"
)

// NewService creates a new service instance
func NewService() *Service {
	return &Service{}
}

// Service is an example of a production ready service implementation
type Service struct{}

// TestErrors will return a variety of errors depending on the req.Case provided.
// Suitable for testing client implementations.
// This method only responds with errors.
func (h *Service) TestErrors(ctx context.Context, req *ErrorsRequest) error {
	switch req.Case {
	case CaseServiceReturnedError:
		return duh.NewServiceError(duh.CodeInternalError, "while reading the database: EOF", nil, nil)
	}

	return nil
}

// TestStream generates a list of items for streaming. If ErrorAt is set, it returns
// an error after generating all items.
func (h *Service) TestStream(ctx context.Context, req *StreamRequest) ([]*StreamItem, error) {
	items := make([]*StreamItem, 0, req.Count)
	for i := int32(0); i < req.Count; i++ {
		items = append(items, &StreamItem{
			Sequence: int64(i),
			Data:     fmt.Sprintf("item-%d", i),
		})
	}

	if req.ErrorAt != "" {
		return items, duh.NewServiceError(duh.CodeInternalError, req.ErrorAt, nil, nil)
	}

	return items, nil
}
