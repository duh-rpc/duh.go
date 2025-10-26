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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/duh-rpc/duh.go"
	v1 "github.com/duh-rpc/duh.go/proto/v1"
	"gopkg.in/yaml.v3"
)

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

type config struct {
	FileName string
	Address  string
	Timeout  string
	Verbose  bool
}

func main() {
	var c config
	f := flag.NewFlagSet("duh", flag.ExitOnError)
	f.StringVar(&c.Address, "address", "localhost:8080",
		"The address of the service in the format '<host|ip>:<port>'")
	f.StringVar(&c.Timeout, "timeout", "10s",
		"The duration to wait for a successful api call (default: 10s)")
	f.BoolVar(&c.Verbose, "verbose", false,
		"be verbose")
	flag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "Usage: %s [flags] [/path/to/file.yaml]\n"+
			"Flags:\n", os.Args[0])
		flag.PrintDefaults()
	}
	checkErr(f.Parse(os.Args[1:]), "while parsing command line args")

	if len(f.Args()) == 0 {
		log.Println("path to yaml file is required")
		f.Usage()
		os.Exit(1)
	}

	c.FileName = f.Args()[0]
	file, err := os.Open(c.FileName)
	checkErr(err, "Error opening file")
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	var buf bytes.Buffer

	var idx int
	for scanner.Scan() {
		if scanner.Text() == "---" {
			process(c, buf.Bytes(), idx)
			buf.Reset()
			idx++
			continue
		}
		buf.WriteString(scanner.Text() + "\n")
	}
	// Process the last document in the file
	process(c, buf.Bytes(), idx)

	// Check for scanner errors
	err = scanner.Err()
	checkErr(err, "while scanning '%s'", c.FileName)
	fmt.Printf("\n")
}

func sendRequest(ctx context.Context, c config, api string, req any) error {
	payload, err := json.Marshal(req)
	if err != nil {
		return duh.NewClientError("while marshaling request payload: %w", err, nil)
	}

	r, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("http://%s%s", c.Address, api), bytes.NewReader(payload))
	if err != nil {
		return duh.NewClientError("", err, nil)
	}
	r.Header.Set("Content-Type", duh.ContentTypeJSON)

	resp, err := duh.DefaultClient.Client.Do(r)
	if err != nil {
		return duh.NewClientError("", err, map[string]string{
			duh.DetailsHttpUrl:    r.URL.String(),
			duh.DetailsHttpMethod: r.Method,
		})
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != duh.CodeOK {
		b, _ := io.ReadAll(resp.Body)
		var reply v1.Reply
		if err := json.Unmarshal(b, &reply); err != nil {
			return duh.NewInfraError(r, resp, b)
		}
		return duh.NewReplyError(r, resp, &reply)
	}

	if c.Verbose {
		_, err := io.Copy(os.Stdout, resp.Body)
		checkErr(err, "while reading response body")
	}

	return nil
}

func process(c config, doc []byte, idx int) {
	if len(bytes.TrimSpace(doc)) == 0 {
		return
	}

	var obj map[string]interface{}
	err := yaml.Unmarshal(doc, &obj)
	checkErr(err, "while parsing YAML at '%d'", idx)

	if c.Verbose {
		jsonData, err := json.MarshalIndent(obj, "", "  ")
		checkErr(err, "error converting YAML to JSON at '%d'", idx)
		fmt.Println(string(jsonData))
	}

	apiAny, ok := obj["api"]
	if !ok {
		fail("expected 'api' field at '%d', but found none", idx)
	}

	api, ok := apiAny.(string)
	if !ok {
		fail("expected 'api' field to be of type string at '%d', but is '%v'", idx, apiAny)
	}

	resource, ok := obj["resource"]
	if !ok {
		fail("expected 'resource' field at '%d', but found none", idx)
	}

	ctx, cancel := context.WithTimeout(context.Background(), parseTimeout(c.Timeout))
	defer cancel()

	err = sendRequest(ctx, c, api, resource)
	checkErr(err, "during sendRequest()")
	fmt.Printf("\n[%d] OK\n", idx)
}

func parseTimeout(s string) time.Duration {
	duration, _ := time.ParseDuration(s)
	return duration
}
