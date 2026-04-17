# The DUH Spec V3
Here we are presenting a general protocol approach to implementing RPC over HTTP. The benefit of using tried-and-true nature of HTTP for RPC allows designers to leverage the many tools and frameworks readily available to design, document, and implement high-performance HTTP APIs without overly complex client or deployment strategies.

When using plain HTTP, a `404 Not Found` could mean a load balancer, API gateway, or proxy couldn't find your service at all — or it could mean your service couldn't find the requested resource. To the client, both look identical. DUH-RPC solves this by using the **Reply structure** as the definitive signal: any response that includes a well-formed Reply body originated from the service; any response that doesn't is treated as infrastructure. This distinction drives how clients interpret errors and whether they should retry.

> The optional `Server: DUH-RPC/1.0` header can help identify service responses when the body isn't available (e.g., in logs), but it may be scrubbed by proxies and should not be relied upon programmatically.

DUH method calls take the form `/v1/problem-domain/subject.method`

The `problem-domain` segment is an optional namespace for grouping related endpoints. Use it when endpoints need organizational separation (e.g., `/v1/billing/invoices.list`, `/v1/identity/sessions.create`). Nesting is permitted. There is no enforcement — it is purely organizational and may span multiple services or API documents.

### Requests
All requests SHOULD use the POST verb with an in body request object describing the arguments of the RPC request. Because each request includes a payload in the encoding of your choice (Default to JSON) there is no need to use  any other HTTP verb other than POST.

> In fact, if you attempt to send a payload using verbs like GET, You will find that some language frameworks assume 
> there is no payload on GET and will not allow you to read the payload.

The name of the RPC method should be in the standard HTTP path such that it can be easily versioned and routed by  standard HTTP handling frameworks, using the form `/<version>/<subject>.<action>`

### Request Body

The client MUST always send a request body, even for methods that define no parameters. For JSON, this means sending `{}`. For Protobuf, a zero-byte body is acceptable, as an empty message unmarshals cleanly.

The server MUST always attempt to unmarshal the request body regardless of whether it is empty. This keeps server-side handling unconditional — no nil-check or missing-body branch is required, and the unmarshalling overhead for an empty message is negligible.

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
* `text/plain` - This should NOT be returned by service implementations. If the response has this content type or has no content type, this indicates to the client the response is not from the service, but from the HTTP infra or some other part of the HTTP stack that is outside of the service implementations control.

> The service implementation MUST always return the content type of the response. It MUST NOT return `text/plain`.

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

The mime types supported can change depending on the method. This allows service implementations to migrate from older  mime types or to support mime types of a specific use case.

If the server can accommodate none of the mime types, the server WILL return code `400` and a standard reply structure with the message  
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

* **Code** (Required) — An application-level signal controlled by the implementor. It MAY be a numeric string
  (e.g. `"400"`, `"453"`) or a semantic string meaningful to the application (e.g. `"CARD_DECLINED"`,
  `"RATE_LIMITED"`). The HTTP status code is the authoritative signal for retry and routing decisions — client
  implementations MUST base retry logic on the HTTP status code, not the `code` field.
* **Message** (Optional) — A human readable message.
* **Details** — (Optional) A map of string key/value pairs providing additional context about this error, which could include a link to documentation explaining the error or additional machine-readable codes.

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
All Server responses MUST ALWAYS return JSON if no other content type is specified. The server WILL NOT return `text/plain`.  This includes any and all router based errors like `Method Not Allowed` or `Not Found` if the route requested is not found. This is because ambiguity exists between a route not found on the service, and a `Not Found` route at the API gateway or Load Balancer.

The server should always respond with a standard **Reply** structure which differentiates its responses from any infrastructure that lies between the service and the client.

The Client SHOULD handle responses that do not include the **Reply** structure and differentiate those responses so as to clearly differentiate between a service `Not Found` replies and infrastructure `Not Found` responses. Implementations of the client SHOULD assume the response is from the infrastructure if it receives a reply that does  NOT conform to the **Reply** structure.

For example, if the client implementation receives an HTTP status code of `404` and a status message of `Not Found` from the request, the client SHOULD assume the error is from the infrastructure and inform the caller in a way that is suitable for the language used.

### Infrastructure Errors
An infrastructure error is any HTTP response code that is NOT 200 and DOES NOT include a `Reply` structure in the body. If the client receives a response code and it DOES NOT include a `Reply` structure in the expected serialization format, then the client MUST consider the response as an infrastructure error and handling it accordingly.

Typically, infrastructure errors are 5XX class errors, but could also be 404 Not Found errors, or consist of 
non-standard or future HTTP status codes. As such it is recommended that client implementations do not attempt to handle all possible HTTP codes, but instead consider any non 200 responses without a `Reply` an infrastructure class
error.

A `404` is the most common example of this ambiguity. A service `404` — one that includes a Reply body — means the requested resource does not exist and should not be retried. An infrastructure `404` — one without a Reply body — means the request never reached the service and SHOULD be retried.

##### Service Identifiers
In addition, the server CAN include the `Server: DUH-RPC/1.0 (Golang)` header according to [RFC9110](https://www.rfc-editor.org/rfc/rfc9110#field.server) to help identify the source of the HTTP Status. (It is possible that proxy or API gateways will scrub or overwrite this header as a security measure, which will make identification of the source more difficult) 

## Retry Semantics

A client SHOULD retry a request if the response meets either of the following conditions:

- The HTTP status code is marked `Retry: True` in the status code table above.
- The response does not include a Reply body — indicating the response originated from infrastructure rather than the service.

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

### oneOf — Discriminated Unions Only
`oneOf` is permitted only when used as a discriminated union — that is, every variant schema MUST include a
discriminator object with a mapping. The discriminator property name is up to the implementor. Bare `oneOf` without a discriminator is not permitted.

Example of a valid discriminated union:
```yaml
oneOf:
  - $ref: '#/components/schemas/CatEvent'
  - $ref: '#/components/schemas/DogEvent'
discriminator:
  propertyName: eventType
  mapping:
    cat: '#/components/schemas/CatEvent'
    dog: '#/components/schemas/DogEvent'
```

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
| `pagination.endCursor`   | string  | Yes      | Cursor to pass as `after` to retrieve the next page |
| `pagination.hasNextPage` | boolean | Yes      | Whether additional results exist beyond this page   |

When `hasNextPage` is `false`, the client SHOULD NOT make a further request. When `hasNextPage` is `true` and `endCursor` is present, the client MAY request the next page by passing `endCursor` as `pagination.after`.

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
