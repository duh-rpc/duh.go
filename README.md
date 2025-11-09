# DUH-RPC - Duh, Use Http for RPC

[![Go Version](https://img.shields.io/github/go-mod/go-version/duh-rpc/duh.go)](https://golang.org/dl/)
[![CI Status](https://github.com/duh-rpc/duh.go/workflows/CI/badge.svg)](https://github.com/duh-rpc/duh.go/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/duh-rpc/duh.go)](https://goreportcard.com/report/github.com/duh-rpc/duh.go)

**DUH-RPC simply says this: You don't need a fancy framework to get the performance of GRPC, just don't do things
with HTTP that are slow**

The JSON and route processing typically used by REST means traditional REST **WILL ALWAYS BE SLOWER THAN RPC**
- JSON marshalling is ALWAYS going to be slower than protobuf
- Regex/Route parsing is ALWAYS going to be slower than matching a string with a switch statement

These two things are the major performance advantages GRPC has over traditional REST.

All you have to do is use protobuf and simple string-based matching like GRPC does. If you do, then any performance
advantages of GRPC are diminished to the point where they don't matter. In fact, as of this writing, DUH-RPC
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
- Others (TODO)

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
* GRPC can be more complex than necessary for high-performance, distributed environments.
* GRPC implementations can be slower than expected (Slower than standard HTTP)
* Using GRPC can result in more code than using standard HTTP
* GRPC is not suitable for public web-based APIs

For a deeper dive and benchmarks of GRPC with standard HTTP in golang, see [Why not GRPC](docs/why-not-grpc.md).

## Why not REST?
Many who embrace RPC-style frameworks do so because they are fleeing REST either because of the simple semantics
of RPC or for performance reasons. In our experience, REST is suboptimal for a few reasons.
* The hierarchical nature of REST does not lend itself to clean interfaces over time.
* REST performance will always be slower than RPC
* No standard error semantics
* No standard streaming semantics
* No standard rate limit or retry semantics

For a deeper dive on REST, see [Why not REST](docs/why-not-rest.md).

## Why build DUH-RPC Style APIs?
* Consistent error handling, which allows libraries and frameworks to handle errors uniformly.
* Consistent RPC method naming, no need to second guess where in the hierarchy the operation should exist.
* You can use the same endpoints and frameworks for both private and public-facing APIs, with no need to have separate
  tooling for each.
* The RPC calls can be interrogated from the command line via curl without the need for a special client.
* The RPC calls can be inspected and called from GUI clients like [Postman](https://www.postman.com/),
  or [hoppscotch](https://github.com/hoppscotch/hoppscotch)
* Use standard schema linting tools and OpenAPI-based services for integration and compliance testing of APIs
* Design, deploy, and generate documentation for your API using standard OpenAPI tools
* Consistent client interfaces allow for a set of standard tooling to be built to support common use cases
  like `retry` and authentication.
* Payloads can be encoded in any format (like ProtoBuf/JSON, MessagePack, Thrift, etc.)

## DUH-RPC Style in a Nutshell
DUH-RPC is a simple RPC-over-HTTP spec using POST-only endpoints.

1. **RPC style path Format**: `/v1/{subject}.{method}`
   - Example: `/v1/users.create`, `/v2/user.get`, `/v1/users.list`
   - Version is major only (v0, v1, v2, etc.)
   - Subject and method: lowercase letters, digits, hyphens, underscores
2. **POST Only** - All operations use POST, even reads
   - No GET, PUT, DELETE, PATCH
3. **Request Body Required**
   - All input data goes in the body
   - No query parameters allowed
4. **Content Types**
   - application/json
   - application/protobuf
5. **Status Codes** - Only these are allowed:
   - 2xx: 200 only
   - 4xx: 400, 401, 403, 429, 452-455
   - 5xx: 500 only
6. **Success Response** - All operations MUST define a 200 response with content
7. **Consistent Error Schema** - All error responses follow a similar schema:
```json
{ 
    "code": 2404,
    "message": "this is the error message",
    "details": {  # optional key-value pairs
        "url": "https://example.com/docs/errors/2404"
        "subcode": "1012.1"
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