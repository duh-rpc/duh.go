package stream

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	FlagData  byte = 0x0
	FlagFinal byte = 0x1
	FlagError byte = 0x2
)

// headerSize is the size of a frame header: 1 byte flag + 4 bytes uint32 length.
const headerSize = 5

// Writer encodes length-prefixed binary frames to an io.Writer.
type Writer struct {
	w io.Writer
}

// NewWriter wraps an io.Writer for frame encoding.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// WriteFrame writes a single frame: [1 byte flag][4 bytes uint32 big-endian length][N bytes payload].
// A final frame with nil/empty payload writes length 0x00000000.
func (w *Writer) WriteFrame(flag byte, payload []byte) error {
	var header [headerSize]byte
	header[0] = flag
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))

	if _, err := w.w.Write(header[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := w.w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// Reader decodes length-prefixed binary frames from an io.Reader.
type Reader struct {
	r              io.Reader
	maxPayloadSize int
}

// NewReader wraps an io.Reader with a maximum payload size. If maxPayloadSize
// is 0, no limit is enforced.
func NewReader(r io.Reader, maxPayloadSize int) *Reader {
	return &Reader{r: r, maxPayloadSize: maxPayloadSize}
}

// ReadFrame reads one frame from the underlying reader. It returns the flag byte,
// the payload, and any error. Returns io.EOF when the reader is exhausted at a
// frame boundary. Returns io.ErrUnexpectedEOF if the reader is exhausted mid-frame.
func (r *Reader) ReadFrame() (flag byte, payload []byte, err error) {
	var header [headerSize]byte
	_, err = io.ReadFull(r.r, header[:])
	if err != nil {
		if err == io.EOF {
			// Exhausted at frame boundary (before reading any header bytes)
			return 0, nil, io.EOF
		}
		// Partial header read (io.ErrUnexpectedEOF from ReadFull)
		return 0, nil, io.ErrUnexpectedEOF
	}

	flag = header[0]
	length := binary.BigEndian.Uint32(header[1:])

	if r.maxPayloadSize > 0 && int(length) > r.maxPayloadSize {
		return 0, nil, fmt.Errorf("frame payload size %d exceeds limit %d", length, r.maxPayloadSize)
	}

	if length == 0 {
		return flag, nil, nil
	}

	payload = make([]byte, length)
	_, err = io.ReadFull(r.r, payload)
	if err != nil {
		// Partial payload read
		return 0, nil, io.ErrUnexpectedEOF
	}

	return flag, payload, nil
}
