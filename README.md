# DUH-RPC - Duh, Use Http for RPC

[![Go Version](https://img.shields.io/github/go-mod/go-version/duh-rpc/duh.go)](https://golang.org/dl/)
[![CI Status](https://github.com/duh-rpc/duh.go/workflows/CI/badge.svg)](https://github.com/duh-rpc/duh.go/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/duh-rpc/duh.go)](https://goreportcard.com/report/github.com/duh-rpc/duh.go)

**DUH-RPC is gRPC without the framework.**

gRPC is fast for two reasons: protobuf instead of JSON, and matching a method string instead of parsing a
route with an HTTP router. DUH-RPC keeps both of the things that makes gRPC fast, BUT drops the framework.
No generated clients, no HTTP/2 trailers, no hidden features. Just protobuf over plain HTTP.

*DUH-RPC simply says this: You don't need a fancy framework to get the performance of GRPC, just don't do slow things
with HTTP!*

#### One client for all services
```go
// gRPC: a generated client per service
greeter := pb.NewGreeterClient(conn)
users   := userpb.NewUsersClient(conn)
greeter.SayHello(ctx, &HelloRequest{...})

// DUH: one call, every endpoint, every service API
client.Post(ctx, "/v1/say.hello",    &HelloRequest{...}, &helloResp)
client.Post(ctx, "/v1/users.create", &CreateUserRequest{...}, &userResp)
```

#### Plain HTTP, no trailers
Both gRPC and DUH use POST with names as the endpoint route, and carrying the same protobuf bytes. gRPC wraps it
in HTTP/2 frames and puts the status in a trailer. This is why curl and browsers can't speak it without a
proxy. DUH is a plain POST any HTTP client already makes

gRPC: HTTP/2 + trailers
```http
POST /say.Greeter/SayHello   HTTP/2
content-type: application/grpc+proto
te: trailers

<5-byte prefix><protobuf bytes>
─────────────────────────────
grpc-status: 0                 (HTTP/2 trailer)
```
DUH-RPC on the Wire is straight HTTP 1.1 or HTTP/2
```
POST /v1/say.hello           HTTP/1.1
Content-Type: application/protobuf

<protobuf bytes>
─────────────────────────────
200 OK
X-DUH-Version: 2.0
```

#### One error shape, everywhere
Every error uses the same Reply structure, so clients, libraries, and even proxies
handle failures uniformly, without custom headers or out-of-band signals:
```json
HTTP/1.1 400

{
  "code":    "CARD_DECLINED",
  "message": "declined due to fraud",
  "details": { ... }
}
```

#### One HTTP API, everywhere
- One transport for internal AND public APIs
- No generated client
- HTTP API Works with Curl, Postman, etc..
- Fully compatible with OpenAPI tooling and docs
- In an apples to apples comparison it can [outperform Go's gRPC implementation](https://github.com/duh-rpc/duh-benchmarks.go)
  
## A DUH-RPC Example Request
`POST http://localhost:8080/v1/say.hello {"name": "John Wick"}`
```json
{
    "message": "Hello, John Wick"
}
```
## Who is using RPC style APIs in production?
- Slack API - Take a look at the [Slack API Methods](https://docs.slack.dev/reference/methods)
- Dropbox API v2 - POST-only RPC endpoints (`/2/files/list_folder`, `/2/users/get_current_account`). Dropbox engineer: ["the API definitely isn't REST"](https://www.dropboxforum.com/t5/Dropbox-API-Support-Feedback/Dropbox-REST-API-mostly-using-POST/td-p/115120)
- Ashby - Public recruiting API uses `/CATEGORY.method` RPC-style paths with POST-only endpoints. They've [written about why they chose RPC](https://medium.com/mergeapi/an-interview-with-ashbys-aaron-norby-on-rpc-apis-f658134597bc) over REST or GraphQL.
- DocMost - API uses `POST /subject/method` style https://docmost.com/api-docs
- Have you seen others? Open a PR, and add it here!

> REMEMBER: You don't need to follow the DUH-RPC spec, just design you own RPC style HTTP API!

## Quick Start
The DUH-RPC cli tool uses OpenAPI to build high-performance RPC-style HTTP endpoints in Go.

Install the `duh` cli linter and generator
```bash
go install github.com/duh-rpc/duh-cli/cmd/duh@latest
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

> REMEMBER: You don't need the CLI to follow the RPC style. The CLI is just a convenient way to generate
> boilerplate code and documentation.

## Why not REST?
Teams adopting RPC are usually fleeing REST. The core problem isn't any single REST decision, it's that
REST gives you *many ways* to do everything, and every choice is a place teams diverge. RPC removes the
decision: there's one way, and it's the same on every endpoint.

| Question          | REST — pick one                                            | DUH-RPC |
|-------------------|------------------------------------------------------------|-------------------|
| Where's the input?| path vars, `?query`, header, form, json body, cookie       | json body         |
| Which verb?       | `GET`, `POST`, `PUT`, `PATCH`, `DELETE`                    | `POST`            |
| What's an error?  | `{error}`, `{message}`, `{detail}`, `{errors:[]}`, per-API | `Reply`           |
| How to paginate?  | offset/limit, page, cursor, `Link` header                  | cursor            |

Two REST-specific costs remain on top of all that divergence:
* REST's hierarchical, resource-oriented design doesn't map cleanly onto operations, and gets awkward over time.
* JSON and a router for REST are slower than protobuf and switch-based routing.

For a deeper dive on REST, see [Why not REST](docs/why-not-rest.md).

## DUH-RPC Style in a Nutshell
DUH-RPC is a simple RPC-over-HTTP spec using POST-only endpoints.

> The DUH-RPC spec is intentionally opinionated. You don't have to adopt it wholesale, but following it means there is less for your team to think about. The conventions are fixed, so developers can focus on building rather than debating API design decisions. Follow the spec and you get consistent, well-structured APIs by default.

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

Also, for structured server-to-client streaming support, See [streaming.md](docs/streaming.md).

For more complete thoughts on gRPC versus standard HTTP in Go, see
[Why not gRPC](docs/why-not-grpc.md).

*Psst: You don't need to follow the spec, JUST use HTTP and build an RPC style API!*
