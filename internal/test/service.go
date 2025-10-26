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
	"github.com/duh-rpc/duh.go"
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
