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
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	v1 "github.com/duh-rpc/duh.go/v2/proto/v1"
	"github.com/duh-rpc/duh.go/v2/stream"
	json "google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// StreamWriter is the server-side interface for writing structured stream frames.
// It is NOT safe for concurrent use.
type StreamWriter interface {
	// Send writes a data frame containing the marshalled message.
	Send(proto.Message) error
	// Close writes a final frame and terminates the stream. If the argument is
	// non-nil, the message is marshalled as the final frame payload. If nil, the
	// final frame has a zero-length payload. Calling Send or Close after Close
	// returns an error.
	Close(proto.Message) error
	// Context returns the request context, used for cancellation checks in the
	// handler loop.
	Context() context.Context
}

var _ StreamWriter = (*streamWriter)(nil)

type streamWriter struct {
	w       *stream.Writer
	flusher http.Flusher
	marshal func(proto.Message) ([]byte, error)
	ctx     context.Context
	closed  bool
}

func (sw *streamWriter) Send(msg proto.Message) error {
	if sw.closed {
		return errors.New("stream is closed")
	}

	payload, err := sw.marshal(msg)
	if err != nil {
		return fmt.Errorf("while marshalling stream message: %w", err)
	}

	if err := sw.w.WriteFrame(stream.FlagData, payload); err != nil {
		return err
	}
	sw.flusher.Flush()
	return nil
}

func (sw *streamWriter) Close(msg proto.Message) error {
	if sw.closed {
		return errors.New("stream is closed")
	}
	sw.closed = true

	var payload []byte
	if msg != nil {
		var err error
		payload, err = sw.marshal(msg)
		if err != nil {
			return fmt.Errorf("while marshalling final stream message: %w", err)
		}
	}

	if err := sw.w.WriteFrame(stream.FlagFinal, payload); err != nil {
		return err
	}
	sw.flusher.Flush()
	return nil
}

func (sw *streamWriter) Context() context.Context {
	return sw.ctx
}

// HandleStream validates the Accept header, asserts http.Flusher, constructs a
// StreamWriter, calls the handler, and handles error frames on handler failure.
func HandleStream(w http.ResponseWriter, r *http.Request, handler func(*http.Request, StreamWriter) error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		ReplyWithCode(w, r, CodeInternalError, nil, "response writer does not support streaming")
		return
	}

	accept := TrimSuffix(r.Header.Get("Accept"), ";,")
	accept = strings.TrimSpace(strings.ToLower(accept))

	var marshalFn func(proto.Message) ([]byte, error)
	switch accept {
	case ContentStreamJSON:
		marshalFn = json.Marshal
	case ContentStreamProtoBuf:
		marshalFn = proto.Marshal
	default:
		ReplyWithCode(w, r, CodeBadRequest, nil,
			fmt.Sprintf("Accept header '%s' is not a supported streaming content type; "+
				"expected '%s' or '%s'", r.Header.Get("Accept"), ContentStreamJSON, ContentStreamProtoBuf))
		return
	}

	w.Header().Set("Content-Type", accept)

	sw := &streamWriter{
		w:       stream.NewWriter(w),
		flusher: flusher,
		marshal: marshalFn,
		ctx:     r.Context(),
	}

	err := handler(r, sw)
	if err == nil {
		return
	}

	// Handler returned an error. If the stream is already closed (final frame
	// was sent), silently give up -- the handler had the Send error and already
	// returned.
	if sw.closed {
		return
	}

	// Construct an error frame from the handler error.
	reply := buildErrorReply(err)
	payload, marshalErr := sw.marshal(reply)
	if marshalErr != nil {
		// Cannot marshal the error reply; nothing more we can do.
		return
	}

	// Write the error frame. If the write fails (client disconnected),
	// silently give up.
	if writeErr := sw.w.WriteFrame(stream.FlagError, payload); writeErr != nil {
		return
	}
	sw.flusher.Flush()
}

// buildErrorReply constructs a v1.Reply from an error. If the error satisfies
// duh.Error, its Code(), Message(), and Details() are used. Otherwise, the
// error is wrapped as a CodeInternalError.
func buildErrorReply(err error) *v1.Reply {
	var e Error
	if errors.As(err, &e) {
		return &v1.Reply{
			Code:    e.Code(),
			Message: e.Message(),
			Details: e.Details(),
		}
	}
	return &v1.Reply{
		Code:    strconv.Itoa(CodeInternalError),
		Message: err.Error(),
	}
}

// StreamReader is the client-side interface for reading structured stream frames.
// It is NOT safe for concurrent use.
type StreamReader interface {
	// Recv reads the next frame from the stream and unmarshals the payload into
	// the provided message. Returns io.EOF when the stream is complete.
	Recv(proto.Message) error
	// Close aborts the in-flight HTTP response and releases resources.
	// Safe to call multiple times.
	Close() error
}

var _ StreamReader = (*streamReader)(nil)

type streamReader struct {
	r          *stream.Reader
	resp       *http.Response
	unmarshal  func([]byte, proto.Message) error
	cancel     context.CancelFunc
	done       bool
	hasPending bool
}

func (sr *streamReader) Recv(msg proto.Message) error {
	// If the stream is already done and there is no pending final payload, return EOF.
	if sr.done && !sr.hasPending {
		return io.EOF
	}

	// If a previous call read a final frame with payload, the payload was already
	// unmarshalled. This call returns EOF to signal stream completion.
	if sr.hasPending {
		sr.hasPending = false
		sr.done = true
		return io.EOF
	}

	flag, payload, err := sr.r.ReadFrame()
	if err != nil {
		if err == io.EOF {
			// Stream ended without a final or error frame -- infrastructure error per spec.
			sr.done = true
			return io.ErrUnexpectedEOF
		}
		sr.done = true
		return err
	}

	switch flag {
	case stream.FlagData:
		return sr.unmarshal(payload, msg)

	case stream.FlagFinal:
		if len(payload) > 0 {
			if err := sr.unmarshal(payload, msg); err != nil {
				sr.done = true
				return err
			}
			sr.hasPending = true
			return nil
		}
		sr.done = true
		return io.EOF

	case stream.FlagError:
		sr.done = true
		var reply v1.Reply
		if err := sr.unmarshal(payload, &reply); err != nil {
			return fmt.Errorf("while unmarshalling error frame: %w", err)
		}
		return &ClientError{
			code:     reply.Code,
			httpCode: sr.resp.StatusCode,
			msg:      reply.Message,
			details:  reply.Details,
		}

	default:
		sr.done = true
		return fmt.Errorf("unknown frame flag: 0x%x", flag)
	}
}

func (sr *streamReader) Close() error {
	if sr.done {
		return nil
	}
	sr.done = true
	sr.cancel()
	return sr.resp.Body.Close()
}
