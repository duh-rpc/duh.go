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
	"context"
	"fmt"

	"github.com/duh-rpc/duh.go"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// NewService creates a new service instance
func NewService() *Service {
	return &Service{}
}

// TODO: Define abstracted errors here
// ErrInternal
// ErrBadRequest
// ErrRequestFailed

// Service is an example of a production ready service implementation
type Service struct{}

func (h *Service) SayHello(ctx context.Context, req *SayHelloRequest, resp *SayHelloResponse) error {
	if req.Name == "" {
		// TODO: This needs to be an internal error, and the caller should instead convert to a NewServiceError
		return duh.NewServiceError(duh.CodeBadRequest, "'name' is required and cannot be empty", nil, nil)
	}
	if cases.Title(language.English).String(req.Name) != req.Name {
		return duh.NewServiceError(duh.CodeBadRequest, "'name' must be capitalized", nil, nil)
	}
	resp.Message = fmt.Sprintf("Hello, %s", req.Name)
	return nil
}

// RenderPixel returns the color of a Mandelbrot fractal at the given point in the image.
// Code copied from Francesc Campoy's Golang Tracer example (https://tinyurl.com/ery6mfz8)
func (h *Service) RenderPixel(ctx context.Context, req *RenderPixelRequest, resp *RenderPixelResponse) error {
	xi := norm(req.I, req.Width, -1.0, 2)
	yi := norm(req.J, req.Height, -1, 1)

	const maxI = 1000
	x, y := 0., 0.

	for i := 0; (x*x+y*y < req.Complexity) && i < maxI; i++ {
		x, y = x*x-y*y+xi, 2*x*y+yi
	}

	resp.Gray = int64(x)
	return nil
}

func norm(x, total int64, min, max float64) float64 {
	return (max-min)*float64(x)/float64(total) - max
}
