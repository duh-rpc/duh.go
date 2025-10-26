package duh

import (
	"fmt"
	v1 "github.com/duh-rpc/duh.go/proto/v1"
	"google.golang.org/protobuf/proto"
	"io"
)

const (
	// International System of Units (SI) Definitions

	Bytes    = 1
	Kibibyte = 1024
	Kilobyte = 1000
	MegaByte = Kilobyte * 1000
	Mebibyte = Kibibyte * 1024
	Gibibyte = Mebibyte * 1024
	Gigabyte = MegaByte * 1024
)

// NewLimitReader returns a ReaderCloser that returns ErrDataLimitExceeded after n bytes have been read.
func NewLimitReader(r io.ReadCloser, n int64) io.ReadCloser {
	return &LimitReader{r: r, remain: n + 1, max: n}
}

type LimitReader struct {
	r      io.ReadCloser
	remain int64
	max    int64
}

func (l *LimitReader) Read(p []byte) (n int, err error) {
	if l.remain <= 0 {
		return 0, &ErrDataLimitExceeded{Max: l.max}
	}
	if int64(len(p)) > l.remain {
		p = p[0:l.remain]
	}
	n, err = l.r.Read(p)
	l.remain -= int64(n)
	return
}

func (l *LimitReader) Close() error {
	return l.r.Close()
}

type ErrDataLimitExceeded struct {
	Max int64
}

func (e *ErrDataLimitExceeded) ProtoMessage() proto.Message {
	return &v1.Reply{
		CodeText: CodeText(CodeBadRequest),
		Code:     CodeBadRequest,
		Message:  e.Message(),
		Details:  nil,
	}
}

func (e *ErrDataLimitExceeded) Code() int {
	return CodeBadRequest
}

func (e *ErrDataLimitExceeded) Message() string {
	return fmt.Sprintf("exceeds %s limit", bytesToSI(e.Max))
}

func (e *ErrDataLimitExceeded) Error() string {
	return e.Message()
}

func (e *ErrDataLimitExceeded) Details() map[string]string {
	return nil
}

func bytesToSI(b int64) string {
	// If divisible by 1024, it's a Kibibyte.
	unit := int64(Kilobyte)
	if b%Kibibyte == 0 {
		unit = int64(Kibibyte)
	}

	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := unit, 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	if unit == Kibibyte {
		return fmt.Sprintf("%.1f%ciB",
			float64(b)/float64(div), "KMGTPE"[exp])
	}
	return fmt.Sprintf("%.1f%cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}
