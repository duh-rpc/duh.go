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
	"errors"
	"fmt"
	"net/http"
	"strconv"

	v1 "github.com/duh-rpc/duh.go/v2/proto/v1"
	"google.golang.org/protobuf/proto"
)

const (
	CodeOK                 = 200
	CodeBadRequest         = 400
	CodeUnauthorized       = 401
	CodeForbidden          = 403
	CodeNotFound           = 404
	CodeConflict           = 409
	CodeTooManyRequests    = 429
	CodeClientError        = 452
	CodeRequestFailed      = 453
	CodeRetryRequest       = 454
	CodeClientContentError = 455
	CodeInternalError      = 500
	CodeNotImplemented     = 501
)

func CodeText(code int) string {
	switch code {
	case CodeOK:
		return "OK"
	case CodeBadRequest:
		return "Bad Request"
	case CodeUnauthorized:
		return "Unauthorized"
	case CodeRequestFailed:
		return "Request Failed"
	case CodeRetryRequest:
		return "Retry Request"
	case CodeForbidden:
		return "Forbidden"
	case CodeNotFound:
		return "Not Found"
	case CodeConflict:
		return "Conflict"
	case CodeClientError:
		return "Client Error"
	case CodeTooManyRequests:
		return "Too Many Requests"
	case CodeInternalError:
		return "Internal Service Error"
	case CodeNotImplemented:
		return "Not Implemented"
	case CodeClientContentError:
		return "Client Content Error"
	default:
		return http.StatusText(code)
	}
}

type Error interface {
	// ProtoMessage creates v1.Reply protobuf from this Error
	ProtoMessage() proto.Message
	// Code returns the application-level code from the Reply body (e.g., "400", "CARD_DECLINED")
	Code() string
	// HTTPCode returns the HTTP status code as an integer
	HTTPCode() int
	// Error is the error message this error wrapped (Used on the server side)
	Error() string
	// Details is the Details of the error retrieved from v1.Reply.details
	Details() map[string]string
	// Message is the message retrieved from v1.Reply.Message
	Message() string
}

var _ Error = (*serviceError)(nil)
var _ Error = (*ClientError)(nil)

type serviceError struct {
	details  map[string]string
	err      error
	code     string
	httpCode int
}

// NewServiceError returns a new serviceError.
// Server Implementations should use this to respond to requests with an error.
// The app code defaults to the string representation of httpCode (e.g., httpCode 400 -> Code() returns "400").
func NewServiceError(httpCode int, msg string, err error, details map[string]string) error {
	return NewServiceErrorWithCode(httpCode, strconv.Itoa(httpCode), msg, err, details)
}

// NewServiceErrorWithCode returns a new serviceError with a custom application-level code
// independent of the HTTP status code (e.g., httpCode 453, code "CARD_DECLINED").
func NewServiceErrorWithCode(httpCode int, code string, msg string, err error, details map[string]string) error {
	if msg != "" {
		if err != nil {
			err = fmt.Errorf(msg, err)
		} else {
			err = errors.New(msg)
		}
	}
	return &serviceError{
		details:  details,
		code:     code,
		httpCode: httpCode,
		err:      err,
	}
}

func (e *serviceError) ProtoMessage() proto.Message {
	return &v1.Reply{
		Message: func() string {
			if e.err != nil {
				return e.err.Error()
			}
			return ""
		}(),
		Code:    e.code,
		Details: e.details,
	}
}

func (e *serviceError) Code() string {
	return e.code
}

func (e *serviceError) HTTPCode() int {
	return e.httpCode
}

func (e *serviceError) Message() string {
	if e.err != nil {
		return e.err.Error()
	}
	return ""
}

func (e *serviceError) Error() string {
	if e.err != nil {
		return CodeText(e.httpCode) + ": " + e.err.Error()
	}
	return CodeText(e.httpCode)
}

func (e *serviceError) Details() map[string]string {
	return e.details
}

type ClientError struct {
	details      map[string]string
	msg          string
	err          error
	isInfraError bool
	code         string
	httpCode     int
}

func (e *ClientError) ProtoMessage() proto.Message {
	msg := e.msg
	if e.err != nil && msg == "" {
		msg = e.err.Error()
	}
	return &v1.Reply{
		Code:    e.code,
		Details: e.details,
		Message: msg,
	}
}

func (e *ClientError) Code() string {
	return e.code
}

func (e *ClientError) HTTPCode() int {
	return e.httpCode
}

func (e *ClientError) IsInfraError() bool {
	return e.isInfraError
}

func (e *ClientError) Message() string {
	return e.msg
}

func (e *ClientError) Error() string {
	if e.err != nil {
		return CodeText(e.httpCode) + ": " + e.err.Error()
	}

	if e.isInfraError {
		return fmt.Sprintf("%s %s returned infrastructure error %d with body: %s",
			e.details[DetailsHttpMethod],
			e.details[DetailsHttpUrl],
			e.httpCode,
			e.msg,
		)
	}

	return fmt.Sprintf("%s %s returned '%s' with message: %s",
		e.details[DetailsHttpMethod],
		e.details[DetailsHttpUrl],
		CodeText(e.httpCode),
		e.msg,
	)
}

func (e *ClientError) Details() map[string]string {
	return e.details
}
