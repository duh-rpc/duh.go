# DUH-RPC - Duh, Use Http for RPC

[![Go Version](https://img.shields.io/github/go-mod/go-version/duh-rpc/duh.go)](https://golang.org/dl/)
[![CI Status](https://github.com/duh-rpc/duh.go/workflows/CI/badge.svg)](https://github.com/duh-rpc/duh.go/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/duh-rpc/duh.go)](https://goreportcard.com/report/github.com/duh-rpc/duh.go)

**DUH-RPC simply says this: You don't need a fancy framework to get the performance of GRPC, just don't do slow things
with HTTP!**

For instance, JSON and route processing typically used by REST means traditional REST **WILL ALWAYS BE SLOWER THAN RPC**
- JSON marshalling is ALWAYS going to be slower than protobuf
- Regex/Route parsing is ALWAYS going to be slower than matching a string with a switch statement

These two things are the major performance advantages GRPC has over traditional REST.

All you have to do is use protobuf and simple string-based route matching like GRPC does. When you do this, 
all the advantages of GRPC are diminished to the point where they don't matter. As of this writing, DUH-RPC
using OpenAPI with standard net/http [outperforms golang GRPC](https://github.com/duh-rpc/duh-benchmarks.go)!

This repo includes a simple implementation of the DUH-RPC spec in golang to illustrate how easy it is to create a high-performance and scalable RPC-style service by following the DUH-RPC spec.

DUH-RPC design is intended to be 100% compatible with OpenAPI tooling, linters, and governance tooling to aid in the
development of APIs without compromising on error handling consistency, performance, or scalability.

## A DUH-RPC Example Request
`POST http://localhost:8080/v1/say.hello {"name": "John Wick"}`
```json
{
    "message": "Hello, John Wick"
}
```
#### Who is using RPC style APIs in production?
- Slack API - Take a look at the [Slack API Methods](https://docs.slack.dev/reference/methods)
- Dropbox API v2 - POST-only RPC endpoints (`/2/files/list_folder`, `/2/users/get_current_account`). Dropbox engineer: ["the API definitely isn't REST"](https://www.dropboxforum.com/t5/Dropbox-API-Support-Feedback/Dropbox-REST-API-mostly-using-POST/td-p/115120)
- Ashby - Public recruiting API uses `/CATEGORY.method` RPC-style paths with POST-only endpoints. They've [written about why they chose RPC](https://medium.com/mergeapi/an-interview-with-ashbys-aaron-norby-on-rpc-apis-f658134597bc) over REST or GraphQL.
- Others? Create a PR!

## Quick Start
The DUH-RPC cli tool uses OpenAPI to build high-performance RPC-style HTTP endpoints
that easily rival or exceed the performance of other RPC frameworks

Install the `duh` cli linter and generator
```bash
go install github.com/duh-cli/duh-cli@latest
```

Get a DUH-RPC service up and running in minutes:

```bash
# 1. Create a new directory for your service
mkdir my-service && cd my-service

# 2. Initialize a new DUH-RPC OpenAPI specification
duh init

# 3. Initialize Go module
go mod init github.com/my-org/my-service

# 4. Generate complete service scaffolding (client, server, daemon, tests, Makefile)
duh generate --full

# 5. Install dependencies
go mod tidy

# 6. Generate Go code from protobuf definitions
buf generate

# 7. Run tests to verify everything works
make test
```

See the [DUH-RPC CLI](https://github.com/duh-rpc/duh-cli) repo for complete documentation on the DUH-RPC CLI.

> NOTE: You don't need the CLI to follow the RPC style. The CLI is just a convenient way to generate
> boilerplate code and documentation.

## Why not GRPC?
GRPC has consistent semantics like flow control, request cancellation, and error handling. However, it is
not without its own issues.
* GRPC requires a generated client for every language — adding a new language means regenerating and distributing clients, whereas HTTP clients are available everywhere out of the box.
* GRPC implementations can be slower than expected in a few situations (Slower than standard HTTP!)
* GRPC is not suitable for public web-based APIs

For a deeper dive and benchmarks of GRPC vs standard HTTP in golang, see [Why not GRPC](docs/why-not-grpc.md).

## Why not REST?
Many who embrace RPC-style frameworks do so because they are fleeing REST due to the simple semantics
of RPC or for performance reasons. In our experience, REST is suboptimal for a few reasons.
* The hierarchical nature of REST does not lend itself to clean interfaces over time.
* REST performance will always be slower than RPC
* No standard error semantics
* No standard streaming semantics
* No standard rate limit or retry semantics

For a deeper dive on REST, see [Why not REST](docs/why-not-rest.md).

## Why build DUH-RPC Style APIs?
* Every endpoint in your entire service mesh is callable with a single, uniform function:
  ```go
  client.Post(ctx, "/v1/hello.world", &HelloRequest{}, &HelloResponse{})
  ```
  Every service, every endpoint, the same call. Error handling, retries, and content negotiation are all handled identically regardless of which service you're talking to.
* Consistent error handling, which allows libraries and frameworks to handle errors uniformly.
* Consistent RPC method naming, no need to second guess where in the Restful hierarchy the operation should exist.
* You can use the same endpoints and frameworks for both private and public-facing APIs — no need to maintain a separate internal transport (gRPC) alongside a public-facing one (REST).
* The RPC calls can be interrogated from the command line via curl without the need for a special client.
* The RPC calls can be inspected and called from GUI clients like [Postman](https://www.postman.com/),
  or [hoppscotch](https://github.com/hoppscotch/hoppscotch)
* Use standard schema linting tools and OpenAPI-based services for integration and compliance testing of APIs
* Design, deploy, and generate documentation for your API using standard OpenAPI tools. Most service catalogs natively support OpenAPI, so building API-first means your services are automatically discoverable and documented — something gRPC alone cannot offer without additional tooling or translation layers.
* Consistent client interfaces allow for a set of standard tooling to be built to support common use cases
  like `retry` and authentication.
* Payloads can be encoded in any format (like ProtoBuf/JSON, MessagePack, Thrift, etc.)
* Because all service responses include the Reply structure, clients can reliably distinguish a service-originated error from one produced by a proxy, gateway, or load balancer — without needing custom headers or out-of-band signals.
* Standard rate limit response semantics, so client libraries can handle backoff uniformly.
* Consistent request body contract — even parameter-less calls send an empty body, eliminating nil-check branches on the server.
* Schema constraints that guarantee protobuf compatibility, enabling seamless code generation.
* Built-in cursor-based pagination semantics for collection endpoints. Because pagination is standardized across every service, you can use a single shared iterator library to page through results from any endpoint in your inventory — wrap your call in a `fetch` function, and the iterator handles cursors, `hasNextPage` checks, and retries automatically:
  ```go
  iter := duh.NewIterator(func(ctx context.Context, cursor string) ([]User, duh.Page, error) {
      resp, err := client.Post(ctx, "/v1/users.list", &ListRequest{Cursor: cursor}, &ListResponse{})
      return resp.Items, resp.Pagination, err
  })
  var page []User
  for iter.Next(ctx, &page) {
      // process page
  }
  if err := iter.Err(); err != nil { ... }
  ```
* Structured server-to-client streaming support — see [streaming.md](docs/streaming.md).

## DUH-RPC Style in a Nutshell
DUH-RPC is a simple RPC-over-HTTP spec using POST-only endpoints.

> The DUH-RPC spec is intentionally opinionated — you don't have to adopt it wholesale, but following it means there is less for your team to think about. The conventions are fixed, so developers can focus on building rather than debating API design decisions. Follow the spec and you get consistent, well-structured APIs by default.

1. **RPC style path Format**: `/v1/{subject}.{method}`
   - Example: `/v1/users.create`, `/v2/user.get`, `/v1/users.list`
   - An optional `problem-domain` namespace segment is supported for grouping: e.g. `/v1/billing/invoices.list`
   - Versioning format is not prescribed — `v1`, `V1`, or `v1.2.1` are all acceptable; versioning in some form is strongly recommended
   - Subject and method: lowercase letters, digits, hyphens, underscores
2. **POST Only** - All operations use POST, even reads
   - No GET, PUT, DELETE, PATCH
3. **Request Body Required**
   - All input data goes in the body
   - No query parameters allowed
4. **Content Types**
   - application/json
   - application/protobuf
   - Additional content types for streaming are supported — see [streaming.md](docs/streaming.md)
5. **Status Codes** - Only these are allowed:
   - 2xx: 200 only
   - 4xx: 400, 401, 403, 404, 409, 429, 452-455
   - 5xx: 500, 501
6. **Success Response** - All operations MUST define a 200 response with content
7. **Consistent Error Schema** - All error responses follow a similar schema. The `code` field is always a string — either a numeric string or a semantic string:
```json
{ 
    "code": "2404",
    "message": "this is the error message",
    "details": {
        "url": "https://example.com/docs/errors/2404",
        "subcode": "1012.1"
    }
}
```
Semantic strings are also valid when there is no appropriate HTTP status code mapping:
```json
{
    "code": "CARD_DECLINED",
    "message": "Credit Card was declined",
    "details": {
        "decline_code": "expired_card",
        "doc": "https://credit.com/docs/card_errors#expired_card"
    }
}
```

#### DUH-RPC is:

- Simple: Fixed conventions eliminate design decisions
- RPC-style: Method calls, not resource manipulation
- Type-safe: Works well with code generation
- Consistent: All operations follow the same pattern

## When is DUH-RPC not appropriate?
DUH-RPC, like most RPC APIs, is intended for service to service communication. It doesn't make sense to use
DUH-RPC if your intended use case is users sharing links to be clicked. Eg.. https://www.google.com/search?q=rpc+api

## The Full DUH-RPC Spec
You can read the full spec [here](docs/spec.md).

*Psst: You don't need to follow the spec, JUST use HTTP and build an RPC style API!*