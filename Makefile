.DEFAULT_GOAL := run

.PHONY: test
test:
	go test -timeout 20m -v -p 1 -race -parallel=1 ./...

.PHONY: proto
proto:
	# Download and install https://buf.build/ before running
	buf generate

.PHONY: run
run:
	go run -race cmd/demo/main.go

.PHONY: lint
lint:
	golangci-lint run -v ./...

.PHONY: tidy
tidy:
	go mod tidy && git diff --exit-code

.PHONY: fmt
fmt:
	go fmt ./... && git diff --exit-code

.PHONY: ci
ci: fmt tidy lint test
	@echo
	@echo "\033[32mEVERYTHING PASSED!\033[0m"

.PHONY: certs
certs: ## Generate SSL certificates
	rm certs/*.key || rm certs/*.srl || rm certs/*.csr || rm certs/*.pem || rm certs/*.cert || true
	openssl genrsa -out certs/ca.key 4096
	openssl req -new -x509 -key certs/ca.key -sha256 -subj "/C=US/ST=TX/O=DUH-RPC Opensource" -days 3650 -out certs/ca.cert
	openssl genrsa -out certs/duh.key 4096
	openssl req -new -key certs/duh.key -out certs/duh.csr -config certs/duh.conf
	openssl x509 -req -in certs/duh.csr -CA certs/ca.cert -CAkey certs/ca.key -set_serial 1 -out certs/duh.pem -days 3650 -sha256 -extfile certs/duh.conf -extensions req_ext
	openssl genrsa -out certs/duh_no_ip_san.key 4096
	openssl req -new -key certs/duh_no_ip_san.key -out certs/duh_no_ip_san.csr -config certs/duh_no_ip_san.conf
	openssl x509 -req -in certs/duh_no_ip_san.csr -CA certs/ca.cert -CAkey certs/ca.key -set_serial 2 -out certs/duh_no_ip_san.pem -days 3650 -sha256 -extfile certs/duh_no_ip_san.conf -extensions req_ext
	# Client Auth
	openssl req -new -x509 -days 3650 -keyout certs/client-auth-ca.key -out certs/client-auth-ca.pem -subj "/C=TX/ST=TX/O=DUH-RPC Opensource/CN=duh.io/emailAddress=admin@duh-rpc.org" -passout pass:test
	openssl genrsa -out certs/client-auth.key 2048
	openssl req -sha1 -key certs/client-auth.key -new -out certs/client-auth.req -subj "/C=US/ST=TX/O=DUH-RPC Opensource/CN=client.com/emailAddress=admin@duh-rpc.org"
	openssl x509 -req -days 3650 -in certs/client-auth.req -CA certs/client-auth-ca.pem -CAkey certs/client-auth-ca.key -set_serial 3 -passin pass:test -out certs/client-auth.pem
	openssl x509 -extfile certs/client-auth.conf -extensions ssl_client -req -days 3650 -in certs/client-auth.req -CA certs/client-auth-ca.pem -CAkey certs/client-auth-ca.key -set_serial 4 -passin pass:test -out certs/client-auth.pem
