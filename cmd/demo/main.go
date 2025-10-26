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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/duh-rpc/duh.go/demo"
)

type config struct {
	Address string
	Verbose bool
}

func checkErr(err error, format string, a ...any) {
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, fmt.Sprintf("%s: %s\n", format, err), a...)
		os.Exit(1)
	}
}

func fail(format string, a ...any) {
	_, _ = fmt.Fprintf(os.Stderr, fmt.Sprintf("%s\n", format), a...)
	os.Exit(1)
}

func main() {
	var c config

	f := flag.NewFlagSet("demo", flag.ExitOnError)
	f.StringVar(&c.Address, "address", "localhost:8080",
		"The address to bind the server to in the format '<host|ip>:<port>'")
	f.BoolVar(&c.Verbose, "verbose", false,
		"be verbose")
	flag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "Usage: %s [flags]\n"+
			"Flags:\n", os.Args[0])
		flag.PrintDefaults()
	}
	checkErr(f.Parse(os.Args[1:]), "while parsing command line args")

	// Create a new instance of our service
	service := demo.NewService()

	server := &http.Server{
		Handler: &demo.Handler{Service: service},
		Addr:    c.Address,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Listening on %s....\n", c.Address)
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fail("ListenAndServe(): %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down the service...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)
	checkErr(err, "during service shutdown")

	log.Println("Service shutdown complete")
}
