# Design Guide
This is the DUH-RPC design guide, while not appart of the specification. provides additional guidance on best 
practices to follow when implementing your own DUH-RPC endpoints.

## Pagination

Pagination allows APIs to return large datasets in manageable chunks, improving performance and user experience. This guide documents cursor-based pagination as the recommended approach for DUH-RPC endpoints.

**Why Cursor-Based Pagination?**

Offset-based pagination (`?offset=100&limit=50`) has performance and security downsides for large datasets. More critically, it suffers from consistency problems when data changes concurrently. For example, if items are added or removed between page requests, users may see duplicate items or miss items entirely.

Cursor-based pagination solves these problems by using opaque cursors that represent a specific position in the dataset. Cursors remain valid even when the underlying data changes, providing a stable pagination experience.

For deeper understanding of pagination patterns and advanced use cases like bidirectional pagination, see the [GraphQL pagination documentation](https://graphql.org/learn/pagination/).

### Core Concepts

**Cursors** are opaque strings representing a position in a dataset. They should be base64-encoded and contain implementation details (like offset, ID, or timestamp) that clients should never parse. This opacity allows backend implementation changes without breaking clients.

**PageInfo** is a message containing pagination metadata: `has_next_page` indicates whether more results exist, and `next_cursor` provides the cursor to fetch the next page.

**Edges** (optional) are wrappers containing both a cursor and a node (the actual item). Use edges when you need per-item cursors or relationship-specific metadata.

**Connection** (optional) is the complete paginated response structure that wraps edges and page_info. Use this pattern when employing edges.

### Schema Conventions

#### Request Schema

Standard paginated list requests use two fields:

```protobuf
message ListUsersRequest {
  string cursor = 1;  // opaque cursor from previous response (empty for first page)
  int32 limit = 2;    // maximum items to return
}
```

#### PageInfo Message

All paginated responses should include page information:

```protobuf
message PageInfo {
  bool has_next_page = 1;  // true if more results exist
  string next_cursor = 2;  // cursor to fetch next page (empty if has_next_page is false)
}
```

#### Simple Pattern (Recommended)

For most use cases, return items directly with page_info:

```protobuf
message ListUsersResponse {
  repeated User users = 1;
  PageInfo page_info = 2;
}

message User {
  string id = 1;
  string name = 2;
  string email = 3;
}
```

**JSON Serialization:**
```json
{
  "users": [
    {"id": "user_1", "name": "Alice", "email": "alice@example.com"},
    {"id": "user_2", "name": "Bob", "email": "bob@example.com"}
  ],
  "page_info": {
    "has_next_page": true,
    "next_cursor": "eyJvZmZzZXQiOjEwMH0="
  }
}
```

#### Connection Pattern (Advanced)

When you need per-item cursors or edge-specific metadata, use the connection pattern:

```protobuf
message UserEdge {
  string cursor = 1;  // cursor for this specific item
  User node = 2;      // the actual item
}

message UsersConnection {
  repeated UserEdge edges = 1;
  PageInfo page_info = 2;
}

message ListUsersResponse {
  UsersConnection connection = 1;
}
```

**JSON Serialization:**
```json
{
  "connection": {
    "edges": [
      {
        "cursor": "eyJpZCI6InVzZXJfMSJ9",
        "node": {"id": "user_1", "name": "Alice", "email": "alice@example.com"}
      },
      {
        "cursor": "eyJpZCI6InVzZXJfMiJ9",
        "node": {"id": "user_2", "name": "Bob", "email": "bob@example.com"}
      }
    ],
    "page_info": {
      "has_next_page": true,
      "next_cursor": "eyJvZmZzZXQiOjEwMH0="
    }
  }
}
```

### Response Structure Reference

**When to use each pattern:**
- **Simple pattern**: Use for standard list endpoints where you only need forward pagination
- **Connection pattern**: Use when you need per-item cursors or edge-specific metadata (e.g., friendship timestamps in social graphs)

**Required fields:**
- `page_info` is required for all paginated responses
- `has_next_page` must accurately reflect whether more results exist
- `next_cursor` must be empty string when `has_next_page` is false

**Empty results:**
```json
{
  "users": [],
  "page_info": {
    "has_next_page": false,
    "next_cursor": ""
  }
}
```

**Error handling:**
- Invalid cursor: Return `400 Bad Request` with message "invalid cursor"
- Expired cursor: Return `400 Bad Request` with message "cursor expired"
- Use standard DUH-RPC error codes and Reply structure

### Iterator Generation Criteria

> This is a planned experimental feature and may change in the future.

The `duh generate http` tool will automatically generate client-side iterators when a response schema contains **all** of the following:

1. **A repeated field** (array of items)
2. **At least one pagination position field**: `cursor`, `pivot`, `page`, `offset`, `has_next_page`
3. **At least one size limit field**: `limit`, `first`, `last`

#### Examples that WILL generate iterators

```protobuf
// Example 1: Simple cursor-based
message ListUsersResponse {
  repeated User users = 1;
  PageInfo page_info = 2;  // contains has_next_page
}
message ListUsersRequest {
  string cursor = 1;
  int32 limit = 2;
}

// Example 2: Direct pagination fields
message ListProductsResponse {
  repeated Product products = 1;
  bool has_next_page = 2;
  string next_cursor = 3;
}
message ListProductsRequest {
  string cursor = 1;
  int32 first = 2;
}

// Example 3: Offset-based (detected but not recommended)
message ListOrdersResponse {
  repeated Order orders = 1;
  int32 total = 2;
}
message ListOrdersRequest {
  int32 offset = 1;
  int32 limit = 2;
}
```

#### Examples that WON'T generate iterators

```protobuf
// Missing pagination fields
message ListUsersResponse {
  repeated User users = 1;
}

// Missing limit field
message ListUsersResponse {
  repeated User users = 1;
  bool has_next_page = 2;
  string next_cursor = 3;
}
message ListUsersRequest {
  string cursor = 1;  // no limit field
}

// Request-only pagination (iterators are only generated from response schemas)
message ListUsersRequest {
  string cursor = 1;
  int32 limit = 2;
}
```

**Detected Pagination Types:**

The tool detects multiple pagination patterns for backward compatibility:
- **Cursor-based**: `cursor` + `limit` (recommended and documented here)
- **Offset-based**: `offset` + `limit` (detected but not recommended)
- **Page-based**: `page` + `limit` (detected but not recommended)
- **GraphQL-style**: `has_next_page` + `first`/`last` (detected)

All generated iterators use the same interface regardless of underlying type.

### Iterator API Design

Generated iterators provide a consistent Go interface for paginating through results:

```go
type UserIterator interface {
    Next(ctx context.Context) (*User, error)
    HasNext() bool
}
```

#### Next() Method

Fetches the next item, automatically handling pagination when needed:

```go
user, err := iter.Next(ctx)
if err == io.EOF {
    // No more items
} else if err != nil {
    // Network error, service error, or invalid cursor
} else {
    // Success: process user
}
```

**Behavior:**
- Returns `(item, nil)` on success
- Returns `(nil, io.EOF)` when no more items exist
- Returns `(nil, error)` on failure (network, invalid cursor, service error)
- Automatically fetches next page when local buffer is exhausted
- Preserves first error encountered; subsequent calls return cached error

#### HasNext() Method

Checks whether more items are available without making network calls:

```go
for iter.HasNext() {
    user, err := iter.Next(ctx)
    if err != nil {
        return err
    }
    // process user
}
```

**Behavior:**
- Returns `true` if more items available (checks `has_next_page` or local buffer)
- Returns `false` when exhausted
- Does not make network calls

#### Context Support

All iterator methods accept `context.Context` for cancellation and deadlines:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

for iter.HasNext() {
    user, err := iter.Next(ctx)
    if err == context.Canceled || err == context.DeadlineExceeded {
        // Context cancelled or timed out
        break
    }
    // process user
}
```

#### Internal Behavior

Iterators track pagination state internally based on detected type:
- **Cursor-based**: Tracks `next_cursor` from `page_info`, sends in `cursor` field of next request
- **Offset-based**: Increments offset counter, sends in `offset` field
- **Page-based**: Increments page counter, sends in `page` field

The same Next()/HasNext() interface works for all pagination types, providing a consistent developer experience.

### Complete Examples

#### Full Protobuf Schema

```protobuf
syntax = "proto3";

package example.v1;

// Request message for listing users
message ListUsersRequest {
  string cursor = 1;  // opaque cursor from previous response (empty for first page)
  int32 limit = 2;    // maximum number of users to return
}

// Response message for listing users
message ListUsersResponse {
  repeated User users = 1;      // the list of users
  PageInfo page_info = 2;       // pagination metadata
}

// Pagination metadata
message PageInfo {
  bool has_next_page = 1;   // true if more results exist
  string next_cursor = 2;   // cursor to fetch next page (empty if has_next_page is false)
}

// User entity
message User {
  string id = 1;       // unique user identifier
  string name = 2;     // user's full name
  string email = 3;    // user's email address
}
```

#### Sample Requests and Responses

**First Page Request:**
```
POST /v1/users.list
Content-Type: application/json

{
  "cursor": "",
  "limit": 100
}
```

**First Page Response:**
```json
{
  "users": [
    {"id": "user_1", "name": "Alice", "email": "alice@example.com"},
    {"id": "user_2", "name": "Bob", "email": "bob@example.com"}
  ],
  "page_info": {
    "has_next_page": true,
    "next_cursor": "eyJvZmZzZXQiOjEwMH0="
  }
}
```

**Next Page Request:**
```
POST /v1/users.list
Content-Type: application/json

{
  "cursor": "eyJvZmZzZXQiOjEwMH0=",
  "limit": 100
}
```

**Last Page Response:**
```json
{
  "users": [
    {"id": "user_201", "name": "Zoe", "email": "zoe@example.com"}
  ],
  "page_info": {
    "has_next_page": false,
    "next_cursor": ""
  }
}
```

#### Client Usage with Generated Iterator

When `duh generate http` detects the pagination pattern, it generates an iterator:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "example.com/api/v1"
)

func main() {
    client := v1.NewClient("https://api.example.com")
    ctx := context.Background()

    // Create iterator with initial request
    iter := client.ListUsers(ctx, &v1.ListUsersRequest{
        Limit: 100,
    })

    // Iterate through all users automatically
    for iter.HasNext() {
        user, err := iter.Next(ctx)
        if err != nil {
            log.Fatal(err)
        }
        fmt.Printf("User: %s <%s>\n", user.Name, user.Email)
    }
}
```

#### Manual Pagination without Iterator

If iterators aren't generated or you need manual control:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "example.com/api/v1"
)

func main() {
    client := v1.NewClient("https://api.example.com")
    ctx := context.Background()

    cursor := ""
    for {
        // Make request with current cursor
        resp, err := client.ListUsers(ctx, &v1.ListUsersRequest{
            Cursor: cursor,
            Limit:  100,
        })
        if err != nil {
            log.Fatal(err)
        }

        // Process users from this page
        for _, user := range resp.Users {
            fmt.Printf("User: %s <%s>\n", user.Name, user.Email)
        }

        // Check if more pages exist
        if !resp.PageInfo.HasNextPage {
            break
        }

        // Update cursor for next page
        cursor = resp.PageInfo.NextCursor
    }
}
```

### Additional Resources

This guide focuses on cursor-based forward-only pagination, which covers the majority of use cases including infinite scroll, batch processing, and data exports.

**For advanced patterns:**
- **Bidirectional pagination** (previous/next navigation): See [GraphQL pagination documentation](https://graphql.org/learn/pagination/)
- **Relay Connection Specification**: Full formal specification at [relay.dev/graphql/connections.htm](https://relay.dev/graphql/connections.htm)

**Cursor Format Recommendations:**

Cursors should be opaque base64-encoded strings. Common internal formats include:
- Offset-based: `base64({"offset": 100})`
- ID-based: `base64({"id": "user_123"})`
- Timestamp-based: `base64({"timestamp": "2025-01-15T12:00:00Z"})`
- Composite: `base64({"created_at": "2025-01-15T12:00:00Z", "id": "user_123"})`

The key is that clients never parse cursors. This implementation detail allows backend changes without breaking clients.

**Scope Note:** This guide documents cursor-based pagination as the recommended pattern. Other pagination approaches (offset-based, page-based) may be detected by `duh generate http` for backward compatibility but are not recommended for new implementations.
