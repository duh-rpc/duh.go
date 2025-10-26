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
	v1 "github.com/duh-rpc/duh.go/proto/v1"
	json "google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"io"
	"net/http"
	"strings"
)

const (
	ContentTypeProtoBuf = "application/protobuf"
	ContentTypeJSON     = "application/json"
	ContentOctetStream  = "application/octet-stream"
	contentPlainText    = "text/plain"
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
			return NewServiceError(e.Code(), fmt.Sprintf("request body %s", e.Message()), nil, nil)
		}
		return NewServiceError(CodeTransportError, "", err, nil)
	}

	// Ignore multiple mime types separated by comma ',' or mime type parameters separated by semicolon ';'
	mimeType := TrimSuffix(r.Header.Get("Content-Type"), ";,")

	switch strings.TrimSpace(strings.ToLower(mimeType)) {
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
		CodeText: CodeText(code),
		Code:     int32(code),
		Details:  details,
		Message:  msg,
	})
}

// ReplyError replies to the request with the error provided. If 'err' satisfies the Error interface,
// then it will return the code and message provided by the Error. If 'err' does not satisfy the Error
// it will then return a status of CodeInternalError with the err.Reply() as the message.
func ReplyError(w http.ResponseWriter, r *http.Request, err error) {
	var re Error
	if errors.As(err, &re) {
		Reply(w, r, re.Code(), re.ProtoMessage())
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
	// Ignore multiple mime types separated by comma ',' or mime type parameters separated by semicolon ';'
	mimeType := TrimSuffix(r.Header.Get("Accept"), ";,")

	switch strings.TrimSpace(strings.ToLower(mimeType)) {
	case "", "*/*", "application/*", ContentTypeJSON:
		b, err := json.Marshal(resp)
		if err != nil {
			// TODO: This should be logged and not returned to the client, we need to define a logger
			ReplyWithCode(w, r, CodeInternalError, nil, err.Error())
			return
		}
		w.Header().Set("Content-Type", ContentTypeJSON)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(code)
		_, _ = w.Write(b)
	case ContentTypeProtoBuf:
		b, err := proto.Marshal(resp)
		if err != nil {
			// TODO: This should be logged and not returned to the client, we need to define a logger
			ReplyWithCode(w, r, CodeInternalError, nil, err.Error())
			return
		}
		w.Header().Set("Content-Type", ContentTypeProtoBuf)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(code)
		_, _ = w.Write(b)
	default:
		r.Header.Set("Accept", ContentTypeJSON)
		ReplyWithCode(w, r, CodeClientContentError, nil, fmt.Sprintf("Accept header '%s' is invalid format "+
			"or unrecognized content type, only [%s] are supported by this method",
			mimeType, strings.Join(SupportedMimeTypes, ",")))
	}
}

// TrimSuffix trims everything after the first separator is found
func TrimSuffix(s, sep string) string {
	if i := strings.IndexAny(s, sep); i >= 0 {
		return s[:i]
	}
	return s
}
