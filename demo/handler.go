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
	"fmt"
	"net/http"

	"github.com/duh-rpc/duh.go/v2"
)

type Handler struct {
	Service *Service
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	// TODO: Middleware
	// TODO: Authentication
	// TODO: Authorization
	// TODO: Max Read Limit Middleware
	// TODO: Rate Limit Middleware

	if r.Method != http.MethodPost {
		duh.ReplyWithCode(w, r, duh.CodeBadRequest, nil,
			fmt.Sprintf("http method '%s' not allowed; only POST", r.Method))
		return
	}

	// No need for fancy routers, a switch case is performant and simple.
	switch r.URL.Path {
	case "/v1/say.hello":
		h.handleSayHello(w, r)
		return
	case "/v1/render.pixel":
		h.handleRenderPixel(w, r)
		return
	case "/v1/events.list":
		duh.HandleStream(w, r, h.listEvents)
		return
	case "/v1/bytes.download":
		h.handleDownloadBytes(w, r)
		return
	}
	duh.ReplyWithCode(w, r, duh.CodeNotImplemented, nil, "no such method; "+r.URL.Path)
}

func (h *Handler) handleSayHello(w http.ResponseWriter, r *http.Request) {
	// TODO: Verify the authenticated user can access this endpoint
	var req SayHelloRequest
	if err := duh.ReadRequest(r, &req, 5*duh.MegaByte); err != nil {
		duh.ReplyError(w, r, err)
		return
	}
	var resp SayHelloResponse
	if err := h.Service.SayHello(r.Context(), &req, &resp); err != nil {
		duh.ReplyError(w, r, err)
		return
	}
	duh.Reply(w, r, duh.CodeOK, &resp)
}

func (h *Handler) listEvents(r *http.Request, stream duh.StreamWriter) error {
	var req ListEventsRequest
	if err := duh.ReadRequest(r, &req, 5*duh.MegaByte); err != nil {
		return err
	}

	events, err := h.Service.ListEvents(r.Context(), &req)
	if err != nil {
		return err
	}

	for _, event := range events {
		if err := stream.Send(event); err != nil {
			return err
		}
	}

	return stream.Close(nil)
}

func (h *Handler) handleDownloadBytes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", duh.ContentOctetStream)
	w.WriteHeader(duh.CodeOK)
	_, _ = w.Write([]byte("hello, bytes"))
}

func (h *Handler) handleRenderPixel(w http.ResponseWriter, r *http.Request) {
	var req RenderPixelRequest
	if err := duh.ReadRequest(r, &req, 5*duh.MegaByte); err != nil {
		duh.ReplyError(w, r, err)
		return
	}
	var resp RenderPixelResponse
	if err := h.Service.RenderPixel(r.Context(), &req, &resp); err != nil {
		duh.ReplyError(w, r, err)
		return
	}
	duh.Reply(w, r, duh.CodeOK, &resp)
}
