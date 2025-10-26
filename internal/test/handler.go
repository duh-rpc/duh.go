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
	"bufio"
	"io"
	"log"
	"math/rand"
	"net/http"

	"github.com/duh-rpc/duh.go"
)

type Handler struct {
	Service *Service
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/test.errors":
		h.handleTestErrors(w, r)
		return
	}
	duh.ReplyWithCode(w, r, duh.CodeNotImplemented, nil, "no such method; "+r.URL.Path)
}

func (h *Handler) handleTestErrors(w http.ResponseWriter, r *http.Request) {
	var req ErrorsRequest

	if err := duh.ReadRequest(r, &req, 0); err != nil {
		duh.ReplyError(w, r, err)
		return
	}

	switch req.Case {
	case CaseInfrastructureError:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(http.StatusText(http.StatusNotFound)))
		return
	case CaseNotImplemented:
		duh.ReplyWithCode(w, r, duh.CodeNotImplemented, nil, "no such method; "+r.URL.Path)
		return
	case CaseClientIOError:
		// Force a chunked response by sending a ton of garbage to the client, this
		// ensures the client will receive response headers such that the panic
		//  abruptly cuts off the stream of chunks.
		if err := chunkWriter(w, 100_000); err != nil {
			log.Printf("during chunkWriter: %s", err)
		}
		panic("Panic in a Handler should terminate the connection")
	}

	duh.ReplyError(w, r, h.Service.TestErrors(r.Context(), &req))
}

func chunkWriter(w io.Writer, length int) error {
	const (
		charset = "abcdefghijklmnopqrstuvwxyz" +
			"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
		size = 4096
	)
	buf := bufio.NewWriter(w)

	for length > 0 {
		chunk := make([]byte, size)
		for i := 0; i < size && length > 0; i++ {
			chunk[i] = charset[rand.Intn(len(charset))]
			length--
		}
		if _, err := buf.Write(chunk); err != nil {
			return err
		}
	}
	return buf.Flush()
}
