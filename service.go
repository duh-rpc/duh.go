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
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	_, _ = w.Write(b)
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
