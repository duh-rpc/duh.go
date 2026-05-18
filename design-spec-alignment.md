# Design: Spec Alignment

This document captures the design decisions for bringing duh.go in line with the
current spec (spec.md). These decisions were reached through discussion and
represent the agreed-upon approach for each area of change.

## Reply Structure Changes

### Reply.Code: int32 to string
The proto `Reply.Code` field changes from `int32` to `string`. The spec allows
Code to be a numeric string (`"453"`) or a semantic string meaningful to the
application (`"CARD_DECLINED"`).

The HTTP status code remains the authoritative signal for retry and routing
decisions. Reply.Code is an application-level signal controlled by the service
implementor.

### Reply.CodeText: Remove
The `codeText` field is removed from the Reply structure entirely. The proto field
`string codeText = 2` is deleted.

`CodeText()` the function remains as an internal utility for formatting error
messages from HTTP status codes. It just no longer appears in the wire format.

## Error Interface Changes

### Code() and HTTPCode() Separation
The `duh.Error` interface gains a distinction between the application-level code
and the HTTP status code.

```go
type Error interface {
    ProtoMessage() proto.Message
    Code() string              // Application-level code from Reply body
    HTTPCode() int             // HTTP status code from the response
    Error() string
    Details() map[string]string
    Message() string
}
```

- `Code()` returns the string code from the Reply body (`"453"`,
  `"CARD_DECLINED"`). For client-side errors (452) and transport errors (512)
  where no Reply body exists, this returns a default string representation.
- `HTTPCode()` returns the HTTP status code as an integer. Retry logic and
  infrastructure error detection operate on this value.

### IsInfraError
A standalone function rather than an interface method, since infrastructure errors
are a client-side concept.

```go
func IsInfraError(err error) bool
```

Performs a type assertion to `*ClientError` and checks the `isInfraError` flag.
Server-side errors are never infrastructure errors.

## Client Changes

### Rate Limit Information
The client surfaces rate limit information from both the Reply body and HTTP
headers. When handling a `429` response:

- `ratelimit-limit`, `ratelimit-remaining`, `ratelimit-reset` are already present
  in `reply.Details` and flow through to the error's `Details()` map.
- The `Retry-After` header value is added to the error's details under a
  `http.retry-after` key so the retry package can access it.

The client does not perform retry itself. It returns the error with all rate limit
metadata attached, and the caller or retry package decides what to do.

## Retry Package Changes

### OnCodes vs OnInfraCodes

The retry policy distinguishes between service errors and infrastructure errors
because the same HTTP status code carries different meaning depending on its
origin.

```go
type Policy struct {
    Interval     Interval
    OnCodes      []int  // Service response codes that trigger retry
    OnInfraCodes []int  // Infrastructure response codes that trigger retry
    Attempts     int
}
```

**OnCodes** are checked when the response includes a Reply body, meaning the
request reached the service and the service returned an error. These are
application-level retryable conditions where the service is explicitly signaling
that the client should try again.

Examples: `429` (rate limited), `454` (retry request), `500` (internal error).

A `404` in `OnCodes` would mean "the service said this resource doesn't exist" --
typically not retryable.

**OnInfraCodes** are checked when the response does NOT include a Reply body,
meaning the request never reached the service or the response came from
infrastructure (load balancer, proxy, API gateway).

Examples: `404` (service not routable), `502` (bad gateway), `503` (no backends),
`504` (gateway timeout).

A `404` in `OnInfraCodes` means "infrastructure couldn't find the service" --
retryable because the service may become reachable.

A nil `OnInfraCodes` means infrastructure errors are not retried. This is useful
for non-idempotent operations where the caller cannot determine if the request was
processed before the infrastructure error occurred.

### Rate Limit Aware Sleep
When retrying a `429`, the retry package inspects the error's `Details()` map for
`ratelimit-reset` or `http.retry-after`. If present, it uses that duration as the
sleep interval instead of the generic backoff. This ensures the client respects
the server's rate limit window rather than hammering with exponential backoff that
may be too aggressive or too conservative.

### Default Policies

```go
// RetryableCodes are service response codes that indicate a transient failure.
var RetryableCodes = []int{429, 454, 500}

// RetryableInfraCodes are infrastructure response codes worth retrying.
var RetryableInfraCodes = []int{404, 502, 503, 504}

// OnRetryable retries indefinitely on known retryable service codes and
// infrastructure errors. Cancel via context.
var OnRetryable = Policy{
    Interval:     DefaultBackOff,
    OnCodes:      RetryableCodes,
    OnInfraCodes: RetryableInfraCodes,
    Attempts:     0,
}
```

## Pagination

### Server Side
Pagination structures are the implementor's responsibility. The spec defines the
contract (cursor-based, `pagination.first`/`pagination.after` in requests,
`items`/`pagination.end_cursor`/`pagination.has_next_page` in responses), but duh.go
does not impose specific protobuf types for service schemas.

### Client Side Iterator
A generic iterator that works with any endpoint following the pagination spec.
The iterator handles cursor management and retry internally.

```go
type Page struct {
    EndCursor   string
    HasNextPage bool
}

type Iterator[T any] struct { /* ... */ }

func NewIterator[T any](
    fetch func(ctx context.Context, cursor string) ([]T, Page, error),
    opts ...IteratorOption,
) *Iterator[T]

func (it *Iterator[T]) Next(ctx context.Context, page *[]T) bool
func (it *Iterator[T]) Err() error
```

- `fetch` is a caller-provided function that wraps their specific RPC call. The
  iterator passes the current cursor; the caller constructs the request with
  their own page size and parameters.
- `Next` populates the provided slice with the next page of results and returns
  true, or returns false when iteration is complete or an error occurs.
- Retry is handled internally using the retry policy provided via options. The
  context passed to `Next` governs both the RPC call and retry cancellation.
- The `Page` metadata (cursors, has_next_page) is internal bookkeeping and is not
  exposed to the caller.

### Usage

```go
iter := duh.NewIterator(func(ctx context.Context, cursor string) ([]User, duh.Page, error) {
    var resp UsersListResponse
    err := client.UsersList(ctx, &UsersListRequest{
        Pagination: &Pagination{First: 50, After: cursor},
    }, &resp)
    return resp.Items, duh.Page{
        EndCursor:   resp.Pagination.EndCursor,
        HasNextPage: resp.Pagination.HasNextPage,
    }, err
}, duh.WithRetryPolicy(retry.OnRetryable))

var page []User
for iter.Next(ctx, &page) {
    db.BulkInsert(ctx, page)
}
if err := iter.Err(); err != nil {
    // handle
}
```

## Deferred

### Streaming
The streaming protocol (`application/duh-stream+json`,
`application/duh-stream+protobuf`, frame format) is specified in `streaming.md`
but implementation is deferred to a separate design and implementation effort.
