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

func TestSIConstants(t *testing.T) {
	assert.Equal(t, int64(1000), int64(duh.Kilobyte))
	assert.Equal(t, int64(1024), int64(duh.Kibibyte))
	assert.Equal(t, int64(1_000_000), int64(duh.MegaByte))
	assert.Equal(t, int64(1_048_576), int64(duh.Mebibyte))
	assert.Equal(t, int64(1_000_000_000), int64(duh.Gigabyte))
	assert.Equal(t, int64(1_073_741_824), int64(duh.Gibibyte))
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

	// Returns the correct SI abbreviations
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
}
