package duh_test

import (
	"bytes"
	"errors"
	"github.com/duh-rpc/duh.go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"strings"
	"testing"
)

// eternalReader is a non-allocating io.Reader that fills any buffer with zero bytes.
// It is used to test large byte-limit boundaries without allocating gigabytes on the heap.
type eternalReader struct{}

func (eternalReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestNewReader(t *testing.T) {
	// Can read up to the requested limit
	r := duh.NewLimitReader(io.NopCloser(strings.NewReader("twenty regular bytes")), 20)
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, out, []byte("twenty regular bytes"))

	// Returns an error if more bytes than requested are read
	r = duh.NewLimitReader(io.NopCloser(strings.NewReader("more than twenty regular bytes")), 20)
	_, err = io.ReadAll(r)
	require.Error(t, err)
	assert.Equal(t, err.Error(), "exceeds 20B limit")

	// Returned error is of type duh.Error
	var d duh.Error
	assert.True(t, errors.As(err, &d))

	// Verify SI vs binary byte constants produce the correct unit labels in error messages.
	// Kilobyte = 1000 (kB), Kibibyte = 1024 (KiB), MegaByte = 1_000_000 (MB), Mebibyte = 1_048_576 (MiB),
	// Gigabyte = 1_000_000_000 (GB), Gibibyte = 1_073_741_824 (GiB).
	r = duh.NewLimitReader(io.NopCloser(bytes.NewReader(make([]byte, duh.Kilobyte*2))), duh.Kilobyte)
	_, err = io.ReadAll(r)
	require.Error(t, err)
	assert.Equal(t, "exceeds 1.0kB limit", err.Error())

	r = duh.NewLimitReader(io.NopCloser(bytes.NewReader(make([]byte, duh.Kibibyte*4))), duh.Kibibyte*3)
	_, err = io.ReadAll(r)
	require.Error(t, err)
	assert.Equal(t, "exceeds 3.0KiB limit", err.Error())

	r = duh.NewLimitReader(io.NopCloser(bytes.NewReader(make([]byte, duh.MegaByte*2))), duh.MegaByte)
	_, err = io.ReadAll(r)
	require.Error(t, err)
	assert.Equal(t, "exceeds 1.0MB limit", err.Error())

	r = duh.NewLimitReader(io.NopCloser(bytes.NewReader(make([]byte, duh.Mebibyte*2))), duh.Mebibyte)
	_, err = io.ReadAll(r)
	require.Error(t, err)
	assert.Equal(t, "exceeds 1.0MiB limit", err.Error())

	r = duh.NewLimitReader(io.NopCloser(eternalReader{}), duh.Gigabyte)
	_, err = io.ReadAll(r)
	require.Error(t, err)
	assert.Equal(t, "exceeds 1.0GB limit", err.Error())

	r = duh.NewLimitReader(io.NopCloser(eternalReader{}), duh.Gibibyte)
	_, err = io.ReadAll(r)
	require.Error(t, err)
	assert.Equal(t, "exceeds 1.0GiB limit", err.Error())
}
