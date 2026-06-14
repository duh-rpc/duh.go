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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	v1 "github.com/duh-rpc/duh.go/v2/proto/v1"
	json "google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	ContentTypeProtoBuf   = "application/protobuf"
	ContentTypeJSON       = "application/json"
	ContentOctetStream    = "application/octet-stream"
	ContentStreamJSON     = "application/duh-stream+json"
	ContentStreamProtoBuf = "application/duh-stream+protobuf"

	HeaderDUHVersion = "X-DUH-Version"
	DUHVersion       = "2.0"
)

var (
	SupportedMimeTypes = []string{ContentTypeJSON, ContentTypeProtoBuf}
)

// ReadRequest reads the given http.Request body []byte into the given proto.Message.
// The provided message must be mutable (e.g., a non-nil pointer to a message).
// It also handles content negotiation via the 'Content-Type' header provided in the http.Request headers
func ReadRequest(r *http.Request, m proto.Message, limit int64) error {
	var b bytes.Buffer
	body := r.Body
	defer func() { _ = body.Close() }()

	if limit > 0 {
		body = NewLimitReader(body, limit)
	}

	_, err := io.Copy(&b, body)
	if err != nil {
		var e Error
		if errors.As(err, &e) {
			return NewServiceError(e.HTTPCode(), fmt.Sprintf("request body %s", e.Message()), nil, nil)
		}
		return NewServiceError(CodeInternalError, "", err, nil)
	}

	switch normalizeMediaType(r.Header.Get("Content-Type")) {
	case "", "*/*", "application/*", ContentTypeJSON:
		if err := json.Unmarshal(b.Bytes(), m); err != nil {
			return NewServiceError(CodeClientContentError, "", err, nil)
		}
		return nil
	case ContentTypeProtoBuf:
		if err := proto.Unmarshal(b.Bytes(), m); err != nil {
			return NewServiceError(CodeClientContentError, "", err, nil)
		}
		return nil
	}
	return NewServiceError(CodeClientContentError, "",
		fmt.Errorf("Content-Type header '%s' is invalid format or unrecognized content type",
			r.Header.Get("Content-Type")), nil)
}

// ReplyWithCode replies to the request with the specified message and status code
func ReplyWithCode(w http.ResponseWriter, r *http.Request, code int, details map[string]string, msg string) {
	Reply(w, r, code, &v1.Reply{
		Code:    strconv.Itoa(code),
		Details: details,
		Message: msg,
	})
}

// ReplyError replies to the request with the error provided. If 'err' satisfies the Error interface,
// then it will return the code and message provided by the Error. If 'err' does not satisfy the Error
// it will then return a status of CodeInternalError with the err.Reply() as the message.
func ReplyError(w http.ResponseWriter, r *http.Request, err error) {
	var re Error
	if errors.As(err, &re) {
		Reply(w, r, re.HTTPCode(), re.ProtoMessage())
		return
	}
	// If err has no Error in the error chain, then reply with CodeInternalError and the message
	// provided.
	ReplyWithCode(w, r, CodeInternalError, nil, err.Error())
}

// Reply responds to a request with the specified protobuf message and status code.
// Reply() provides content negotiation for protobuf if the request has the 'Accept' header set.
// If no 'Accept' header was provided, Reply() will marshall the proto.Message into JSON.
func Reply(w http.ResponseWriter, r *http.Request, code int, resp proto.Message) {
	mimeType := normalizeMediaType(r.Header.Get("Accept"))

	var marshalFn func(proto.Message) ([]byte, error)
	var contentType string
	switch mimeType {
	case "", "*/*", "application/*", ContentTypeJSON:
		marshalFn = json.Marshal
		contentType = ContentTypeJSON
	case ContentTypeProtoBuf:
		marshalFn = proto.Marshal
		contentType = ContentTypeProtoBuf
	default:
		r.Header.Set("Accept", ContentTypeJSON)
		ReplyWithCode(w, r, CodeClientContentError, nil, fmt.Sprintf("Accept header '%s' is invalid format "+
			"or unrecognized content type, only [%s] are supported by this method",
			mimeType, strings.Join(SupportedMimeTypes, ",")))
		return
	}

	b, err := marshalFn(resp)
	if err != nil {
		ReplyWithCode(w, r, CodeInternalError, nil, err.Error())
		return
	}
	w.Header().Set("Content-Type", contentType)
	setServiceHeaders(w)
	w.WriteHeader(code)
	_, _ = w.Write(b)
}

// ReadContent reads the raw request body and returns the bytes, the raw Content-Type header
// value, and any error. Unlike ReadRequest, the Content-Type is not normalized — content
// endpoints may need MIME parameters like charset=utf-8.
// A limit of 0 means no limit is applied. Empty bodies and empty Content-Type are not errors.
func ReadContent(r *http.Request, limit int64) ([]byte, string, error) {
	var b bytes.Buffer
	body := r.Body
	defer func() { _ = body.Close() }()

	if limit > 0 {
		body = NewLimitReader(body, limit)
	}

	_, err := io.Copy(&b, body)
	if err != nil {
		var e Error
		if errors.As(err, &e) {
			return nil, "", NewServiceError(e.HTTPCode(), fmt.Sprintf("request body %s", e.Message()), nil, nil)
		}
		return nil, "", NewServiceError(CodeInternalError, "", err, nil)
	}

	return b.Bytes(), r.Header.Get("Content-Type"), nil
}

// WriteContent writes a raw content response with the given Content-Type and body.
// It sets X-DUH-Version and X-Content-Type-Options headers via setServiceHeaders.
func WriteContent(w http.ResponseWriter, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	setServiceHeaders(w)
	w.WriteHeader(CodeOK)
	_, _ = w.Write(body)
}

// BytesWriter is the server-side interface for writing an unstructured
// (octet-stream) response body, the counterpart to the client's DoBytes and the
// unstructured analogue of StreamWriter. A handler can set response headers
// (including overriding the default Content-Type) and write body bytes, but it
// cannot commit the status code: HandleBytes defers WriteHeader until the first
// Write so a handler that fails before producing any output still yields a
// standard error Reply. Set any headers before the first Write.
type BytesWriter interface {
	Header() http.Header
	io.Writer
}

// bytesWriter is the default BytesWriter. It defers the 200 status until the
// first Write and, when the ResponseWriter supports it, flushes each write so
// bytes stream to the client as they are produced. Flushing is best-effort: an
// unflushable writer still produces a correct response, just buffered.
type bytesWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	wrote   bool
}

func (b *bytesWriter) Header() http.Header {
	return b.w.Header()
}

func (b *bytesWriter) Write(p []byte) (int, error) {
	if !b.wrote {
		b.w.WriteHeader(CodeOK)
		b.wrote = true
	}
	n, err := b.w.Write(p)
	if err == nil && b.flusher != nil {
		b.flusher.Flush()
	}
	return n, err
}

// HandleBytes writes an unstructured (octet-stream) response, the server-side
// counterpart to the client's DoBytes and the unstructured analogue of
// HandleStream. It seeds Content-Type: application/octet-stream (the handler MAY
// override it via BytesWriter.Header() before the first Write), runs handler, and
// — if the handler returns an error before any bytes were written — sends a
// standard error Reply. Once bytes have been written the 200 response is committed
// and the error can only abort the stream. See docs/streaming.md.
func HandleBytes(w http.ResponseWriter, r *http.Request, handler func(*http.Request, BytesWriter) error) {
	w.Header().Set("Content-Type", ContentOctetStream)
	setServiceHeaders(w)
	flusher, _ := w.(http.Flusher)
	bw := &bytesWriter{w: w, flusher: flusher}
	if err := handler(r, bw); err != nil && !bw.wrote {
		ReplyError(w, r, err)
	}
}

// ReplyContentError writes an error Reply for content endpoint errors.
// It delegates to ReplyError so the Accept header is respected and the version header is set.
// This function exists as a named entry point for content endpoint handlers, parallel to
// ReplyError for structured handlers, allowing future content-specific behavior.
func ReplyContentError(w http.ResponseWriter, r *http.Request, err error) {
	ReplyError(w, r, err)
}

// setServiceHeaders sets the standard DUH service response headers.
// Called by Reply and WriteContent to ensure all service responses
// include the version header and content-type options.
func setServiceHeaders(w http.ResponseWriter) {
	w.Header().Set(HeaderDUHVersion, DUHVersion)
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// TrimSuffix trims everything after the first separator is found
func TrimSuffix(s, sep string) string {
	if i := strings.IndexAny(s, sep); i >= 0 {
		return s[:i]
	}
	return s
}

// normalizeMediaType strips MIME parameters (after ';' or ','), trims whitespace,
// and lowercases the result, producing a canonical media type for comparison.
func normalizeMediaType(value string) string {
	return strings.TrimSpace(strings.ToLower(TrimSuffix(value, ";,")))
}
