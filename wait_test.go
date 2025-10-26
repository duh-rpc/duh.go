package duh_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/duh-rpc/duh.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"log"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestWaitForConnectTLS(t *testing.T) {
	tlsConf := duh.TLSConfig{
		CaFile:   "certs/ca.cert",
		CertFile: "certs/duh.pem",
		KeyFile:  "certs/duh.key",
	}
	err := duh.SetupTLS(&tlsConf)
	require.NoError(t, err)

	srv := http.Server{
		Addr: "localhost:9685",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintln(w, "Hello, client")
		}),
		ErrorLog:  log.New(io.Discard, "", log.LstdFlags),
		TLSConfig: tlsConf.ServerTLS,
	}
	defer func() { _ = srv.Close() }()

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := srv.ListenAndServeTLS("", "")
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Logf("server listen error: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	err = duh.WaitForConnect(ctx, "localhost:9685", tlsConf.ClientTLS)
	defer cancel()
	assert.NoError(t, err)
}

func TestWaitForConnect(t *testing.T) {
	srv := http.Server{
		Addr: "localhost:9685",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintln(w, "Hello, client")
		}),
	}
	defer func() { _ = srv.Close() }()

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Logf("server listen error: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	err := duh.WaitForConnect(ctx, "localhost:9685", nil)
	defer cancel()
	assert.NoError(t, err)
}
