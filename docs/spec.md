# The DUH Spec V2
Here we are presenting a general protocol approach to implementing RPC over HTTP. The benefit of using tried-and-true nature of HTTP for RPC allows designers to leverage the many tools and frameworks readily available to design, document, and implement high-performance HTTP APIs without overly complex client or deployment strategies.

When using plain HTTP, a `404 Not Found` could mean a load balancer, API gateway, or proxy couldn't find your service at all — or it could mean your service couldn't find the requested resource. To the client, both look identical. DUH-RPC solves this with two signals: the **`X-DUH-Version` header** on every service response, and the **Reply structure** in error response bodies. If a response includes `X-DUH-Version`, it came from the service. If it does not, it came from infrastructure. This distinction drives how clients interpret errors and whether they should retry.

The server MUST include the `X-DUH-Version` header on every response, including 200, 4xx, and 5xx. The value is the DUH-RPC spec version the service implements (e.g. `X-DUH-Version: 2.0`). Unlike the `Server` header, custom headers are not scrubbed by proxies, making this a reliable signal.

DUH method calls take the form `/v1/problem-domain/subject.method`

The `problem-domain` segment is an optional namespace for grouping related endpoints. Use it when endpoints need organizational separation (e.g., `/v1/billing/invoices.list`, `/v1/identity/sessions.create`). Nesting is permitted. There is no enforcement — it is purely organizational and may span multiple services or API documents.

### Requests
All requests SHOULD use the POST verb with an in body request object describing the arguments of the RPC request. Because each request includes a payload in the encoding of your choice (Default to JSON) there is no need to use  any other HTTP verb other than POST.

> In fact, if you attempt to send a payload using verbs like GET, You will find that some language frameworks assume 
> there is no payload on GET and will not allow you to read the payload.

The name of the RPC method should be in the standard HTTP path such that it can be easily versioned and routed by  standard HTTP handling frameworks, using the form `/<version>/<subject>.<action>`

### Request Body

The client MUST always send a request body, even for methods that define no parameters. For JSON, this means sending `{}`. For Protobuf, a zero-byte body is acceptable, as an empty message unmarshals cleanly.

The server MUST always read the request body, regardless of whether it is empty. For structured endpoints, this means unmarshaling into the request schema; an empty JSON body (`{}`) or zero-byte protobuf body unmarshals cleanly into an empty message. For content endpoints, this means reading the raw bytes. This keeps server-side handling unconditional — no nil-check or missing-body branch is required, and the unmarshalling overhead for an empty message is negligible.

CRUD Examples
* `/v1/users.create`
* `/v1/users.delete`
* `/v1/users.update`
  Where `.` denotes that the action occurs on the `users` collection.

Methods SHOULD always reference the thing they are acting upon
* `/v1/dogs.pet`
* `/v1/dogs.feed`

If Methods do not act upon a collection, then you should indicate the subject the method is acting on
* `/v1/messages.send`
* `/v1/trash.clear`
* `/v1/kiss.give`

Naming it `/v1/kiss.give` instead of just `/v1/kiss` is an important future proofing action. In case you want to add other methods to `/v1/kiss`, you have a consistent API. For instance if you only had `/v1/kiss` then when you  wanted to add the ability to `blow` a kiss, your api would look like

* `/v1/kiss` - Create a Kiss
* `/v1/kiss.blow` - Blow a Kiss

Instead of the more consistent
* `/v1/kiss.give` - Give a Kiss
* `/v1/kiss.blow` - Blow a Kiss

### Subject before Noun or Action
You may have noticed that every endpoint has the subject before the action. This is intentional and is useful for  future proofing your API when you add more actions in the future. Just remember, if in doubt, design your API like Yoda would speak.

### Versioning
DUH-RPC calls employ standard HTTP paths to make RPC calls. This approach makes versioning those methods easy and direct. This spec does NOT have an opinion on the versioning semantic used. IE: `v1` or `V1` or `v1.2.1`. It is highly recommended that all methods are versioned in some way.

## Content Negotiation
This spec defines support for only the following mime types which can be specified in the `Content-Type` or `Accept`  headers. However, since this is HTTP, service Implementors are free to add support for whatever mime type they want.

* `application/json` - This MUST be used when sending or receiving JSON. The charset MUST be UTF-8
* `application/protobuf` - This MUST be used when sending or receiving Protobuf. The charset MUST be ascii
* `application/octet-stream` - This MUST be used when sending or receiving unstructured binary data. The charset is undefined. The client/server should receive and store the binary data in its unmodified form.
* `application/duh-stream+json` - This MUST be used when sending or receiving a structured server-to-client stream with JSON encoded payloads. See [streaming.md](streaming.md).
* `application/duh-stream+protobuf` - This MUST be used when sending or receiving a structured server-to-client stream with Protobuf encoded payloads. See [streaming.md](streaming.md).
> The service implementation MUST always return the content type of the response.

> These content types are used by structured endpoints. Content endpoints, where the request or response body is opaque content, use the content's own MIME type. The JSON fallback rule ("server MUST ALWAYS support JSON") applies to structured and error responses, not to the content body of a content endpoint. See [Content Endpoints](#content-endpoints).

#### Content-Type and Accept Headers
Clients SHOULD NOT specify any mime type parameters as specified in RFC2046.  Any parameters after `;` in the provided content type CAN be ignored by the server.

The Content Type is expected to be specified in the following format, omitting any mime type parameters like `;charset=utf-8` or `;q=0.9`.

```
Content-Type: <MIME_type>/<MIME_subtype>
```

> Multiple mime types are not allowed, `Content-Type` and `Accept` header MUST NOT contain multiple mime types separated by comma as defined in [RFC7231](https://www.rfc-editor.org/rfc/rfc7231#section-5.3.2) If multiple mime types are 
> provided, the server will ignore any mime type beyond the first provided or may optionally ignore the Content-Type completely and return JSON. 
>
> Implementations that add new mime types are encouraged to also follow this rule as it simplifies client  and server implementations as the RFC style of negotiation is unnecessary within the scope of RPC.
>
> The no-parameters rule applies to structured endpoints. Content endpoints MAY use fully qualified MIME types with parameters (e.g. `text/html; charset=utf-8`) as the content's own MIME type may require them. DUH-RPC passes these through to the implementation without interpretation. See [Content Endpoints](#content-endpoints).

The mime types supported can change depending on the method. This allows service implementations to migrate from older  mime types or to support mime types of a specific use case.

If the server can accommodate none of the mime types, the server WILL return HTTP status code `455` and a standard reply structure with the message  
```
Accept header 'application/bson' is invalid format or unrecognized content type, only 
[application/json, application/protobuf] are supported by this method
```

> Server MUST ALWAYS support JSON, if no other content type can be negotiated, then the server will always respond with JSON.

The reason we simplify Content-Type handling here is so servers with high performance requirements or tight resource constraints can support the DUH-RPC spec without needing to support every edge case RFC for HTTP.

## Replies
Standard replies from the service SHOULD follow a common structure. This provides a consistent and simple method for clients to reply with errors and messages.

#### Reply Structure
The reply structure has the following fields.

* **Code** (Optional) — An application-level signal controlled by the implementor. It MAY be a numeric string
  (e.g. `"400"`, `"453"`) or a semantic string meaningful to the application (e.g. `"CARD_DECLINED"`,
  `"RATE_LIMITED"`). The HTTP status code is the authoritative signal for retry and routing decisions — client
  implementations MUST base retry logic on the HTTP status code, not the `code` field.
* **Message** (Optional) — A human readable message.
* **Details** (Optional) — A map of string key/value pairs providing additional context about this error, which could include a link to documentation explaining the error or additional machine-readable codes.

All fields are optional. A Reply MAY contain only `code`, only `message`, only `details`, or any combination. The presence of a well-formed Reply structure (even an empty one) is what distinguishes a service response from an infrastructure response — see [Infrastructure Errors](#infrastructure-errors).

> Although the **Reply** structure is typically used for error replies, it CAN be used in normal `200` responses when there is a desire to avoid adding a new `<MethodCall>Response` type for simple method call which has no detailed responses.

### Errors
Errors are returned using the Reply structure. The HTTP status code is the authoritative signal for retry and
routing decisions. The `code` field in the reply body is an application-level signal and is not required to mirror
the HTTP status code.

#### HTTP Status Codes

| Code | Short                | Retry | Long                                                                              |
|------|----------------------|-------|-----------------------------------------------------------------------------------|
| 200  | OK                   | N/A   | Everything is fine                                                                |
| 400  | Bad Request          | False | Client is missing a required parameter, or value provided is invalid              |
| 401  | Unauthorized         | False | Not Authenticated, Who are you?  (AuthN)                                          |
| 403  | Forbidden            | False | You can't access this thing, or you don't have authorization (AuthZ)              |
| 404  | Not Found            | False | The thing you where looking for was not found                                     |
| 409  | Conflict             | False | This request conflicts with another request                                       |
| 429  | Too Many Requests    | True  | Stop abusing our service. (See Standard RateLimit Responses)                      |
| 453  | Request Failed       | False | Request is valid, but failed. If no other code makes sense, use this one          |
| 454  | Retry Request        | True  | Request is valid, but service asks the client to try again.                       |
| 455  | Client Content Error | False | Something about the content the client provided is wrong (Not following DUH spec) |
| 500  | Internal Error       | True  | Something with our server happened that is out of our control                     |
| 501  | Not Implemented      | False | The method requested is not implemented on this server                            |

>  Most Standard HTTP Clients will handle 1xx and 3xx class errors, so we don't include those here.

###### Service HTTP Codes
When you break it down by what implementation is responsible for what HTTP code, it breaks down like this.

Service Implementation - 200, 400, 404, 409, 453, 454, 500
DUH-RPC Implementation - 455, 501

> `452` is a client-side code synthesized by the client SDK. It is never received over the wire. See [Client-Side Codes](#client-side-codes).
AuthZ/AuthN Implementation - 401, 403
RateLimit Implementation - 429

#### Errors and Codes
HTTP Status Codes should NOT be expanded for your specific use case. Instead, server implementations should add their own custom fields and codes in the standard `v1.Reply.Details` map.

For example, a credit card processing company needs to return card processing errors. The recommended path would be to add those `details` to the standard error.
```json
{
    "code": "453",
    "message": "Credit Card was declined",
    "details": {
        "type": "card_error",
        "code": "CARD_DECLINED",
        "decline_code": "expired_card",
        "doc": "https://credit.com/docs/card_errors#expired_card"
    }
}
```

The `code` field can also carry a semantic string directly when there is no appropriate HTTP status code mapping:
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

It is NOT recommended to add your own custom fields to the **Reply** structure. This approach would require clients to use a non-standard structure. For example, the following is not recommended.
```json
{
    "code": "453",
    "message": "Credit Card was declined",
    "error": {
        "description": "expired_card",
        "department": {
            "id": "0x87DD0006",
            "sub_code": "1003002",
            "type": "E102"
        }
    },
    "details": {
        "doc": "https://credit.com/docs/80070045D/E102#1003002"
    }
}
```

#### Client-Side Codes

`452 Client Error` is a synthetic code generated by the client SDK. It is never sent over the wire by a server and will never appear in the HTTP status code table. It signals that the request failed before a response was received, or that the response could not be interpreted.

A client SDK SHOULD return `452` in the following scenarios:

| Scenario | Description |
|---|---|
| Marshal failure | The request payload could not be serialized (e.g. a protobuf struct contains invalid UTF-8). The request was never sent. |
| Request construction failure | The HTTP request object could not be created (e.g. the method string is invalid). The request was never sent. |
| Transport failure | `http.Client.Do()` failed before a response was received (e.g. DNS failure, connection refused, unsupported scheme). |
| Response deserialization failure | The server returned `200` but the response body could not be unmarshalled. |

The first three scenarios are deterministic local failures — retrying will produce the same result. The fourth is ambiguous: it may indicate a client/server schema mismatch or a server bug. Retry is still incorrect, but implementations SHOULD surface this case distinctly from the others so it can be investigated. A deserialization failure on a `200` is a signal to examine the server, not just fix the client.

**Retry: False** for all `452` scenarios.

#### Handling HTTP Errors
All Server responses MUST ALWAYS return JSON if no other content type is specified. Content endpoints return the content's own MIME type on 200; error responses follow the existing Content Negotiation rules. See [Content Endpoints](#content-endpoints). This includes any and all router based errors like `Method Not Allowed` or `Not Found` if the route requested is not found. This is because ambiguity exists between a route not found on the service, and a `Not Found` route at the API gateway or Load Balancer.

The server MUST include the `X-DUH-Version` header and a standard **Reply** structure on all error responses.

### Infrastructure Errors
An infrastructure error is any HTTP response that does NOT include the `X-DUH-Version` header **and** has an HTTP status code of 5xx or 404. These are the status codes that infrastructure components typically produce when a request fails to reach the service.

A missing `X-DUH-Version` header on other status codes (e.g. 200, 400, 401, 403) indicates a non-conforming service, not an infrastructure error.

The `X-DUH-Version` header check is a last-resort signal. Client implementations SHOULD first evaluate the HTTP status code, content type, and response body using the normal rules. Only when no other rule applies and the status code is 5xx or 404 should the client check for the absence of `X-DUH-Version` to classify the response as an infrastructure error.

A `404` is the most common example of this ambiguity. A service `404` includes the `X-DUH-Version` header and a Reply body, meaning the requested resource does not exist and should not be retried. An infrastructure `404` has no `X-DUH-Version` header, meaning the request never reached the service and SHOULD be retried.

## Retry Semantics

A client SHOULD retry a request if the response meets any of the following conditions:

- The HTTP status code is marked `Retry: True` in the status code table above.
- The response is an infrastructure error (see [Infrastructure Errors](#infrastructure-errors)).

In both cases, the client SHOULD retry with backoff. The backoff strategy and maximum retry duration are left to the implementor.

### Infrastructure Errors and Idempotency

When a request fails with an infrastructure error, the client cannot determine whether the request was received and processed by the service before the error occurred. Retrying is still the correct default behavior, but non-idempotent operations (e.g. `.create`, `.send`) carry an inherent risk of duplication.

If a service wants to make non-idempotent operations safely retryable, it SHOULD provide an idempotency key mechanism. If no such mechanism is provided, the client SHOULD NOT retry non-idempotent operations that fail with an infrastructure error.

## RPC Call Semantics
An RPC call or request over the network has the following semantics

### CRUD Semantics
Similar to REST semantics
`/subject.get` always returns data and does not cause side effects.
`/subject.create` creates a thing and can return the data it just created
`/subject.update` updates an existing resource
`/subject.delete` deletes an existing resource

##### Request reached service and the client received a response
The response from the service could be good or bad. No interruptions occurred that would have impeded the request from reaching the service which handles the request.

##### Request was rejected
The request could be rejected by the service or infrastructure for various reasons.
* IP Blacklisted
* Rate Limited
* No such Endpoint or Method found
* Not Authenticated
* Malformed or Incorrect request parameters
* Not Authorized

#### Request timed out
The request could have timed due to some infrastructure issue or some client or service saturation event.
* Upstream service timeout (DB or other service timeout)
* TCP idle timeout rule (proxy or firewall)
* 502 Gateway Timeout

##### Request is cancelled by the service
The request could have been cancelled by
* Service shutdown during processing
* Catastrophic Failure of the server or service

##### The caller cancels the request
The request timed out waiting for a reply, or the caller requested a cancel of the request while in flight.

##### Request was denied by infrastructure
The infrastructure attempted to connect the request with a service, but was denied
* 503 Service Unavailable (Load Balancer has no upstream backends to fulfill the request)
* Internal Error on the Load Balancer or Proxy
* TCP firewall drops SYN (Possibly due to SYN Flood, and silently never connects)

### RPC Call Characteristics
Based on the above possible outcomes we can infer that an RPC client should have the following characteristics.
##### Unary RPC Requests should be retried until canceled
Because a request could time out, be rejected, canceled, or denied for transient reasons, the client should have some resiliency built-in and retry depending on the error received. The request should continue to retry with back off until the client determines the request took too long, or the client cancels the request.

The retryable status codes and the conditions under which a client should retry are defined in the [Retry Semantics](#retry-semantics) section.

##### Stream Requests

A stream that disconnects before any frames are received is an infrastructure error. The same retry rules as unary requests apply.

A stream that disconnects after one or more data frames, without a final or error frame, is also an infrastructure error. The client MAY retry from the beginning. Whether a full retry is safe depends on the idempotency of the stream endpoint — the same principle that applies to non-idempotent unary operations applies here. If the stream endpoint encodes sequence information in the payload, the client SHOULD pass the last received sequence value in the retry request so the server can resume from the correct position. See [streaming.md](streaming.md) for details on the sequence-in-payload pattern.

A stream that terminates with a final or error frame MUST NOT be retried. The stream ended intentionally.

## Rate Limit Responses

This section applies only when the service itself is responsible for rate limiting. If rate limiting is handled entirely by infrastructure (e.g. an API gateway), this section does not apply and no Reply body will be present.

When a service returns a `429 Too Many Requests` response, it MUST include a Reply body to distinguish it from an infrastructure-level rate limit. The Reply body SHOULD include a human-readable message explaining the rate limit.

```json
{
  "code": "429",
  "message": "Rate limit exceeded, please slow down",
  "details": {
    "ratelimit-limit": "1000",
    "ratelimit-remaining": "0",
    "ratelimit-reset": "30"
  }
}
```

| Field                 | Required | Description                                                          |
| --------------------- | -------- | -------------------------------------------------------------------- |
| `ratelimit-limit`     | No       | Total number of requests allowed in the current window               |
| `ratelimit-remaining` | No       | Requests remaining in the current window                             |
| `ratelimit-reset`     | No       | Seconds until the rate limit resets. Supports decimals (e.g. `0.4`) |

Services SHOULD include a `Retry-After` header alongside the Reply body, using the delay-seconds form, for compatibility with infrastructure and HTTP clients that handle rate limiting at the transport level.

A `429` that does not include a Reply body MUST be treated as an infrastructure-level rate limit. The client SHOULD retry using the same backoff strategy defined in the [Retry Semantics](#retry-semantics) section.

## Schema Design

### Request and Response Schemas
Every operation MUST define a dedicated request schema and a dedicated response schema. Schemas MUST NOT be shared across operations. This ensures each operation has a clear, unambiguous contract and maps cleanly to generated code and protobuf message definitions.

By convention, schemas are named after the operation they belong to, e.g. `UserCreateRequest` and `UserCreateResponse`, though any consistent naming convention is acceptable (camelCase, snake_case, or kebab-case). Tooling that generates protobuf definitions will normalize names to the appropriate convention.

### Streaming Endpoints

A streaming endpoint is identified by its response `Content-Type`. An endpoint whose response declares `application/duh-stream+json` or `application/duh-stream+protobuf` is a streaming endpoint. The response schema defines the single payload type carried by all data and final frames on that stream. See [streaming.md](streaming.md).

### No readOnly / writeOnly Fields
Service schemas MUST NOT use `readOnly` or `writeOnly` on individual properties. These annotations imply a single schema is shared between a request and a response context, which violates the dedicated schema rule above. If a field only appears in a response, it belongs only in the response schema. If a field only appears in a request, it belongs only in the request schema.

## Protobuf Compatibility

DUH-RPC natively supports `application/protobuf` as a content type. To ensure schemas can be represented as protobuf messages without loss of fidelity, service schemas SHOULD observe the following constraints.

### No Nullable Fields
Protobuf has no native concept of a null value. Fields should not be marked `nullable: true`. Use the absence of a field (i.e. optional fields) to represent the lack of a value rather than an explicit null.

### Integer and Number Format Required
All integer and number fields MUST specify an explicit format. Protobuf requires knowing the exact numeric type at compile time. Use `int32`, `int64`, `float`, or `double` as appropriate. An unformatted `integer` or `number` field is ambiguous and cannot be mapped to a protobuf scalar.

### No Nested Arrays
Protobuf does not support repeated repeated fields. Schemas MUST NOT define arrays whose items are themselves arrays. If nested collection semantics are required, wrap the inner array in a message type.

### Typed additionalProperties
Protobuf maps require explicit key and value types. When using `additionalProperties` to represent a map, the value type MUST be explicitly specified (e.g. `additionalProperties: { type: string }`). Untyped `additionalProperties` cannot be mapped to a protobuf map field.

### No allOf or anyOf
`allOf` has no equivalent in protobuf and MUST NOT be used. `anyOf` introduces ambiguous typing that cannot be represented in protobuf and MUST NOT be used.

### oneOf — Nested Key-Tagged Unions Only
`oneOf` is permitted only as a nested, key-tagged union: an object with one optional `$ref` property per
variant plus a `oneOf` of single-`required` branches and **no `discriminator`**. The present key names the
variant and its payload nests beneath it (`{"cat_event": {"pet_name": "Whiskers"}}`). This is the one form
that serializes identically on both the JSON and protobuf wires, because it maps directly to a protobuf
`oneof`. See ADR-0002.

Example of a valid nested union:
```yaml
Event:
  type: object
  properties:
    cat_event:
      $ref: '#/components/schemas/Cat'
    dog_event:
      $ref: '#/components/schemas/Dog'
  oneOf:
    - required: [cat_event]
    - required: [dog_event]
```

The **discriminated/flat `oneOf`** — a top-level `oneOf` of `$ref`/inline variants plus a `discriminator` —
is **NOT permitted**. It hoists the selected variant's fields to the top level and tags them by value
(`{"eventType": "cat", "pet_name": "Whiskers"}`), a shape no protobuf serialization can produce.

Each variant branch MUST name exactly one declared property, and that property MUST NOT be an array
(proto3 forbids a `repeated` field inside a `oneof`).

## Content Endpoints

When the payload is the content (an HTML document, an image, a PDF) wrapping it in a JSON string field forces the client to escape every quote and newline in the document or encode as base64. Content endpoints let the client POST and receive content directly, in its native MIME type, without an intermediate serialization layer.

A content endpoint is any endpoint where the request body, the response body, or both carry opaque content rather than a structured message.

### Metadata Headers

On structured endpoints, parameters go in the request body. On content endpoints the body is the content, so request parameters MUST be sent as HTTP headers.

Metadata headers SHOULD use the `X-RPC-` prefix followed by the parameter name. Any header starting with `X-RPC-` is a parameter for the RPC call. Standard HTTP headers (`Content-Type`, `Content-Length`, `Authorization`) retain their normal meaning.

The server MAY include `X-RPC-` headers in responses to return metadata about the operation (e.g. `X-RPC-Version`, `X-RPC-Hash`).

```
X-RPC-Path: /engineering/three-personas
X-RPC-Author: agent-writer-01
X-RPC-Format: html
```

The OpenAPI definition for the endpoint declares each metadata header as a parameter with `in: header`.

### Requests

`Content-Type` declares the content format. The client MUST send a body; the body is the content. The server reads it as raw bytes.

```
POST /v1/pages.upload HTTP/1.1
Content-Type: text/html
X-RPC-Path: /engineering/three-personas
X-RPC-Author: agent-writer-01

<!DOCTYPE html>
<html lang="en">
<head><title>Three Personas</title></head>
<body>...</body>
</html>
```

> A content endpoint whose request body is structured (e.g. a download that takes a JSON query) follows the normal structured request rules for that body. Only the content body is treated as raw bytes.

### Responses

On 200, `Content-Type` declares the content format and the body is raw content. On non-200, the response MUST include a standard Reply body. A non-200 without a Reply is an infrastructure error, same as any other endpoint.

Structured request, content response:
```
POST /v1/pages.download HTTP/1.1
Content-Type: application/json

{"path": "/engineering/three-personas"}
```
```
HTTP/1.1 200 OK
Content-Type: text/html
X-Content-Type-Options: nosniff

<!DOCTYPE html>
<html lang="en">
<head><title>Three Personas</title></head>
<body>...</body>
</html>
```

Content request, structured response:
```
POST /v1/pages.upload HTTP/1.1
Content-Type: text/html
X-RPC-Path: /engineering/three-personas

<!DOCTYPE html>
...
```
```
HTTP/1.1 200 OK
Content-Type: application/json

{"path": "/engineering/three-personas", "size": 48230, "version": 3}
```

The request and response do not need to use the same content type. A content request MAY produce a structured response, and a structured request MAY produce a content response.

### Errors

Error responses MUST include a Reply body. The Reply encoding follows the existing Content Negotiation rules.

```
HTTP/1.1 400 Bad Request
Content-Type: application/json

{"code": "400", "message": "X-RPC-Path header is required"}
```
```
HTTP/1.1 404 Not Found
Content-Type: application/json

{"code": "404", "message": "page not found: /engineering/three-personas"}
```

### Content Types

The endpoint MUST define which content types it accepts and returns.

## Pagination

For endpoints that return collections, DUH-RPC uses cursor-based forward pagination. Offset and limit style pagination is not supported. Cursor-based pagination is preferred because it scales consistently regardless of dataset size and does not suffer from the page-drift problem inherent in offset pagination.
### Detecting Paginated Endpoints
An endpoint is considered paginated if its response schema consists of an `items` array and a `pagination` object. Endpoints whose action implies a list (e.g. `.list`, `.search`) and whose response contains an array field or a `total` count SHOULD be paginated.
### Request Structure
Paginated requests MUST include pagination parameters nested under a `pagination` sub-object in the request body:

| Field              | Type    | Required | Description                                         |
| ------------------ | ------- | -------- | --------------------------------------------------- |
| `pagination.first` | integer | Yes      | Number of items to return. Minimum: 1, Maximum: 100 |
| `pagination.after` | string  | No       | Cursor indicating the position to start from        |

### Response Structure
Paginated responses MUST include the following structure:

| Field                    | Type    | Required | Description                                         |
| ------------------------ | ------- | -------- | --------------------------------------------------- |
| `items`                  | array   | Yes      | The page of results                                 |
| `pagination.end_cursor`   | string  | Yes      | Cursor to pass as `after` to retrieve the next page |
| `pagination.has_next_page` | boolean | Yes      | Whether additional results exist beyond this page   |

When `has_next_page` is `false`, the client SHOULD NOT make a further request. When `has_next_page` is `true` and `end_cursor` is present, the client MAY request the next page by passing `end_cursor` as `pagination.after`.

### Prohibited Pagination Patterns
The following parameter names are prohibited as they imply offset-style pagination: `limit`, `offset`, and `page` as a standalone parameter name (i.e. a top-level integer page number). The `pagination` sub-object described above is the only permitted use of pagination parameters.

### Backwards Pagination
Forward-only pagination (`first`/`after`) is the baseline requirement. Backwards pagination (`last`/`before`) MAY be added to an endpoint without violating this spec, provided the forward pagination fields under `pagination` remain present.

### FIN
If you got this far, go look at the `demo/client.go` and `demo/service.go` for examples of an implementation in golang.

# DEMO

### Only POST is allowed
`GET http://localhost:8080/v1/say.hello`
```json
{
  "code": "400",
  "message": "http method 'GET' not allowed; only POST"
}
```

### Missing a request body
`POST http://localhost:8080/v1/say.hello`
```json
{
  "code": "455",
  "message": "proto: syntax error (line 1:1): unexpected token "
}
```
> **Note:** This behavior is a bug in the reference implementation. Per the [Request Body](#request-body) section, the server MUST always attempt to unmarshal the request body, and an empty body MUST be treated as equivalent to an empty message. A missing body should not produce a `455`.

### Validation error
`POST http://localhost:8080/v1/say.hello {"name": ""}`
```json
{
  "code": "400",
  "message": "'name' is required and cannot be empty"
}
```
