# DUH-RPC - Duh, Use Http for RPC

[![Go Version](https://img.shields.io/github/go-mod/go-version/duh-rpc/duh.go)](https://golang.org/dl/)
[![CI Status](https://github.com/duh-rpc/duh.go/workflows/CI/badge.svg)](https://github.com/duh-rpc/duh.go/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/duh-rpc/duh.go)](https://goreportcard.com/report/github.com/duh-rpc/duh.go)

**DUH-RPC uses OpenAPI and all the tooling that comes with it to build high-performance RPC-style HTTP endpoints
that easily rival or exceed the performance of other RPC frameworks**

The JSON and route processing typically used by REST means traditional REST **WILL ALWAYS BE SLOWER THAN RPC**
- JSON marshalling is always going to be slower than protobuf
- Regex/Route parsing is always going to be slower than matching a string with a switch statement

These two things are the major performance advantages GRPC has over traditional REST.

If you use protobuf and simple string-based matching like GRPC does, then any performance
advantages of GRPC are diminished to the point where they don't matter. In fact, as of this writing, DUH-RPC
using OpenAPI and standard net/http [outperforms golang GRPC](https://github.com/duh-rpc/duh-benchmarks.go)!

DUH-RPC simply says this: You don't need a fancy framework to get the performance of GRPC, just don't do things
that are slow. 

This repo includes a simple implementation of the DUH-RPC spec in golang to illustrate how easy it is to create a high-performance and scalable RPC-style service by following the DUH-RPC spec.

DUH-RPC design is intended to be 100% compatible with OpenAPI tooling, linters, and governance tooling to aid in the
development of APIs without compromising on error handling consistency, performance, or scalability.

## A Simple DUH-RPC Example
`POST http://localhost:8080/v1/say.hello {"name": "John Wick"}`
```json
{
    "message": "Hello, John Wick"
}
```

> TODO: Use duh-cli to install and create a new demo project, then demo linting and DUH-RPC code generation from an
OpenAPI spec.

## When is DUH-RPC not appropriate?
DUH-RPC, like most RPC APIs, is intended for service to service communication. It doesn't make sense to use
DUH-RPC if your intended use case is users sharing links to be clicked. Eg.. https://www.google.com/search?q=rpc+api

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

### What's good about the DUH-RPC Spec
* Consistent error handling, which allows libraries and frameworks to handle errors uniformly.
* Consistent RPC method naming, no need to second guess where in the hierarchy the operation should exist.
* You can use the same endpoints and frameworks for both private and public-facing APIs, with no need to have separate
  tooling for each.
* Keeps the good parts of REST: stateless, cacheable, intermediates, security
* The API can be interrogated from the command line via curl without the need for a special client.
* The API can be inspected and called from GUI clients like [Postman](https://www.postman.com/),
  or [hoppscotch](https://github.com/hoppscotch/hoppscotch)
* Use standard schema linting tools and OpenAPI-based services for integration and compliance testing of APIs
* Design, deploy and generate documentation for your API using standard OpenAPI tools
* Consistent client interfaces allow for a set of standard tooling to be built to support common use cases
  like `retry` and authentication.
* Payloads can be encoded in any format (like ProtoBuf, MessagePack, Thrift, etc.)


## The DUH-RPC Spec
You can read all the details of the spec [here](docs/spec.md).

## Existing RPC Options
There are already plenty of frameworks to choose from:
* GRPC
* https://dubbo.apache.org
* [GRPC Web](https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-WEB.md)
* [DRPC](https://github.com/storj/drpc)

