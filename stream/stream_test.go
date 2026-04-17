package stream_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/duh-rpc/duh.go/v2/stream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteReadRoundTrip(t *testing.T) {
	// Write a data frame, read it back
	var buf bytes.Buffer
	w := stream.NewWriter(&buf)
	r := stream.NewReader(&buf, 0)

	require.NoError(t, w.WriteFrame(stream.FlagData, []byte("hello")))
	flag, payload, err := r.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, stream.FlagData, flag)
	assert.Equal(t, []byte("hello"), payload)

	// Write a final frame with payload, read it back
	buf.Reset()
	require.NoError(t, w.WriteFrame(stream.FlagFinal, []byte("goodbye")))
	flag, payload, err = r.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, stream.FlagFinal, flag)
	assert.Equal(t, []byte("goodbye"), payload)

	// Write a final frame with nil payload (length 0), read it back
	buf.Reset()
	require.NoError(t, w.WriteFrame(stream.FlagFinal, nil))
	flag, payload, err = r.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, stream.FlagFinal, flag)
	assert.Nil(t, payload)

	// Write an error frame, read it back
	buf.Reset()
	require.NoError(t, w.WriteFrame(stream.FlagError, []byte("error details")))
	flag, payload, err = r.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, stream.FlagError, flag)
	assert.Equal(t, []byte("error details"), payload)

	// Write multiple frames sequentially, read them all back in order
	buf.Reset()
	require.NoError(t, w.WriteFrame(stream.FlagData, []byte("frame1")))
	require.NoError(t, w.WriteFrame(stream.FlagData, []byte("frame2")))
	require.NoError(t, w.WriteFrame(stream.FlagData, []byte("frame3")))
	require.NoError(t, w.WriteFrame(stream.FlagFinal, nil))

	flag, payload, err = r.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, stream.FlagData, flag)
	assert.Equal(t, []byte("frame1"), payload)

	flag, payload, err = r.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, stream.FlagData, flag)
	assert.Equal(t, []byte("frame2"), payload)

	flag, payload, err = r.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, stream.FlagData, flag)
	assert.Equal(t, []byte("frame3"), payload)

	flag, payload, err = r.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, stream.FlagFinal, flag)
	assert.Nil(t, payload)

	// Large payload (64KB) — verify no corruption
	buf.Reset()
	large := make([]byte, 64*1024)
	for i := range large {
		large[i] = byte(i % 256)
	}
	require.NoError(t, w.WriteFrame(stream.FlagData, large))
	flag, payload, err = r.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, stream.FlagData, flag)
	assert.Equal(t, large, payload)
}

func TestReadFrameErrors(t *testing.T) {
	for _, test := range []struct {
		name    string
		input   []byte
		maxSize int
		wantErr error
		errMsg  string
	}{
		{
			name:    "ReaderExhaustedAtFrameBoundary",
			input:   []byte{},
			maxSize: 0,
			wantErr: io.EOF,
		},
		{
			name:    "ReaderExhaustedAfterPartialHeader",
			input:   []byte{0x00, 0x00, 0x00},
			maxSize: 0,
			wantErr: io.ErrUnexpectedEOF,
		},
		{
			name: "ReaderExhaustedMidPayload",
			// Flag=0x00, Length=10 (but only 3 bytes of payload follow)
			input:   []byte{0x00, 0x00, 0x00, 0x00, 0x0A, 0x01, 0x02, 0x03},
			maxSize: 0,
			wantErr: io.ErrUnexpectedEOF,
		},
		{
			name: "PayloadLengthExceedsMax",
			// Flag=0x00, Length=100
			input:   []byte{0x00, 0x00, 0x00, 0x00, 0x64},
			maxSize: 50,
			errMsg:  "exceeds limit",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := stream.NewReader(bytes.NewReader(test.input), test.maxSize)
			_, _, err := r.ReadFrame()
			require.Error(t, err)
			if test.wantErr != nil {
				assert.ErrorIs(t, err, test.wantErr)
			}
			if test.errMsg != "" {
				assert.ErrorContains(t, err, test.errMsg)
			}
		})
	}

	// maxPayloadSize of 0 — no limit enforced, large frames succeed
	t.Run("NoLimitLargeFrameSucceeds", func(t *testing.T) {
		var buf bytes.Buffer
		w := stream.NewWriter(&buf)
		large := make([]byte, 128*1024)
		require.NoError(t, w.WriteFrame(stream.FlagData, large))

		r := stream.NewReader(&buf, 0)
		flag, payload, err := r.ReadFrame()
		require.NoError(t, err)
		assert.Equal(t, stream.FlagData, flag)
		assert.Len(t, payload, 128*1024)
	})
}
