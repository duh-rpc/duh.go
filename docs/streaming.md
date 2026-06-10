## Streaming

HTTP handles request/response well. It handles streams less cleanly, not because the protocol can't do it, but because there are several ways to do it and they differ in ways that matter. DUH-RPC defines two streaming content types with distinct semantics. Picking the right one is a function of your payload type, not personal preference.

| Content Type | Encoding | Use Case |
|---|---|---|
| `application/octet-stream` | Raw bytes | Unstructured binary transfer |
| `application/duh-stream+json` | JSON | Server-to-client structured stream |
| `application/duh-stream+protobuf` | Protobuf | Server-to-client structured stream |

All streaming endpoints MUST use POST, same as any other DUH-RPC method.

---

### Unstructured Streams

`application/octet-stream` transfers raw bytes with no application-level framing. Use it for file downloads, binary exports, or any payload where the content is opaque to the service layer.

Error handling is straightforward. Once bytes start flowing there is no safe place to inject a Reply structure. Errors MUST be returned as a standard Reply response before any bytes are sent. A `200` response with `Content-Type: application/octet-stream` means the body is raw bytes; the client MUST NOT attempt to parse it as a Reply.

A mid-stream disconnection is an infrastructure error. The client MAY retry from the beginning.

Resumable transfers are supported via the standard HTTP `Range` and `Content-Range` headers. The server MAY support range requests; the client MAY request a byte range on retry. See [RFC 7233](https://www.rfc-editor.org/rfc/rfc7233) for full semantics.

---

### Structured Streams

Structured streaming sends a sequence of typed messages over a single HTTP response. Two content types support this. `application/duh-stream+json` carries JSON payloads and `application/duh-stream+protobuf` carries Protobuf payloads. They share the same conceptual model and wire format; only the payload encoding differs.

A structured streaming endpoint carries exactly one payload type. The response schema defines that type. Mixing payload types on a single stream is not permitted.

#### Frame Semantics

Every frame has one of three meanings:

| Concept | Flag |
|---|---|
| Data frame | `0x0` |
| Final frame | `0x1` |
| Error frame | `0x2` |
| Heartbeat frame | `0x3` |

A **data frame** carries a single instance of the stream's payload type.

A **final frame** signals a clean end of stream. It MAY carry a payload of the same type as data frames. If the server has nothing meaningful to include, the payload MUST be empty.

An **error frame** signals that the stream has terminated due to an error. Its payload is always a standard Reply structure, encoded in the stream's content type. After sending an error frame the server MUST close the stream. The client MUST NOT expect further frames.

A **heartbeat frame** is a keep-alive signal sent by the server when the stream is idle. Its payload MUST be empty (length `0`), making it exactly 5 bytes on the wire: `[0x03][0x00][0x00][0x00][0x00]`. The handler never sends heartbeat frames directly; the framework sends them automatically. Clients MUST silently consume heartbeat frames — they MUST NOT be surfaced to application code.

> The error frame and heartbeat frame are exceptions to the single payload type rule. They are distinguishable by their flag values, so there is no ambiguity.

After a final or error frame, the stream is closed. Sending additional frames after either is a protocol violation. Heartbeat frames MUST NOT be sent after a final or error frame.

#### Structured Streams
Are made up of two different encodings, while using the same framing. (`application/duh-stream+json` and `application/duh-stream+protobuf`) Both content types use length-prefix framing. The payload encoding (JSON or Protobuf) is determined by the content type declared in the `Accept` header of the request. Each frame is structured as follows:

```
[ 1 byte: flag ][ 4 bytes: unsigned 32-bit big-endian length ][ N bytes: payload ]
```

The flag byte values are:

| Value | Meaning |
|---|---|
| `0x0` | Data frame |
| `0x1` | Final frame |
| `0x2` | Error frame |
| `0x3` | Heartbeat frame |

The length field specifies the byte length of the payload that follows. For a final frame with no payload, length MUST be `0`. For an error frame, the payload is a Reply structure encoded in the content type negotiated for the stream. For a heartbeat frame, length MUST be `0`.

Receivers that encounter an unknown flag value SHOULD skip the frame (read and discard the payload bytes indicated by the length field) rather than treating it as an error. This allows future flag values to be added without breaking existing clients.

The `Content-Type` header on the response echoes the content type requested by the client in the `Accept` header, either `application/duh-stream+json` or `application/duh-stream+protobuf`. The client MUST use this value as the authoritative signal for how to decode frame payloads.

> The JSON fallback rule from the general spec (where a server that cannot satisfy the requested content type falls back to `application/json`) does **not** apply to streaming endpoints. If the client requests `application/duh-stream+protobuf` and the server does not support it, the server MUST return a `400` with a standard Reply structure. A fallback to `application/duh-stream+json` is not permitted; the client has already committed to a binary encoding and cannot be expected to handle a different stream format.

A mid-stream disconnection before a final or error frame is an infrastructure error. The client MAY retry from the beginning. Resumption from a specific frame is not supported; if your use case requires it, encode sequence information in your payload type.

#### Heartbeats

Structured streams support server-initiated heartbeats to keep idle connections alive through intermediaries (proxies, load balancers, firewalls) and to let clients detect half-open TCP connections.

**Server behavior.** The server SHOULD send heartbeat frames at a regular interval when the stream is idle. The default interval is 30 seconds. The server MAY configure a different interval via `StreamConfig.HeartbeatInterval`. A negative interval disables heartbeats entirely; a zero value uses the default.

Heartbeat frames are sent by a background goroutine that shares a mutex with `Send` and `Close`. A heartbeat frame never interleaves with a partially-written data, final, or error frame. When `Close` is called (or the handler returns an error), the heartbeat goroutine is stopped before the final or error frame is written.

**Client behavior.** The client MUST silently consume heartbeat frames. When `Recv` encounters a heartbeat frame, it resets its internal deadline and reads the next frame — the caller never sees the heartbeat.

If the client has a heartbeat timeout configured (default 60 seconds), it starts a timer before each `ReadFrame` call. If no frame of any kind arrives within the timeout, `Recv` returns `ErrHeartbeatTimeout` — a sentinel error distinct from `io.ErrUnexpectedEOF` and `io.EOF`. Every received frame (data, final, error, or heartbeat) resets the timeout clock. A negative timeout disables enforcement; a zero value uses the default.

The heartbeat timeout SHOULD be at least 2× the server's heartbeat interval. The client has no protocol-level mechanism to discover the server's interval; the 2× convention is a safe default.

A heartbeat frame on the wire:
```
// Heartbeat (5 bytes total, no payload)
flag=0x3  length=0x00000000
```

Heartbeat frames interspersed with data frames — the client sees only the data frames:
```
// Frame 1: data
flag=0x0  length=0x00000031
{"sequence": 1, "userId": "abc", "action": "login"}

// Heartbeat (invisible to client)
flag=0x3  length=0x00000000

// Frame 2: data
flag=0x0  length=0x00000032
{"sequence": 2, "userId": "def", "action": "logout"}

// Final
flag=0x1  length=0x00000000
```

A payload type with a sequence field looks like this:
```
// Frame 1: data
flag=0x0  length=0x00000031
{"sequence": 1, "userId": "abc", "action": "login"}

// Frame 2: data
flag=0x0  length=0x00000032
{"sequence": 2, "userId": "def", "action": "logout"}
```
On reconnect, the client sends the last received `sequence` value in the request body. The server resumes from that point. The framing layer has no knowledge of this; it is entirely between the client and server application logic.

A final frame carrying an example payload of the same type:
```
// Frame 42: final, with payload
flag=0x1  length=0x00000021
{"sequence": 42, "total": 42}
```

A final frame with no payload:
```
flag=0x1  length=0x00000000
```

An error frame arriving after data frames have already been sent. The client MUST be prepared to receive an error frame at any point in the stream, not just at the start:
```
// Frame 1: data
flag=0x0  length=0x00000031
{"sequence": 1, "userId": "abc", "action": "login"}

// Frame 2: error (stream terminates here)
flag=0x2  length=0x00000045
{"code": "500", "message": "upstream source failed at record 2"}
```

#### A note on `text/event-stream` and `EventSource`

SSE and the browser `EventSource` API were considered as the browser streaming transport for DUH-RPC and rejected for two reasons:

- It is limited to GET requests and cannot carry a request body. Stream parameters would need to be passed as query strings, exposing potentially sensitive data in URLs and server logs.
- SSE was designed for server-push use cases (unsolicited updates from server to browser). DUH-RPC streams are parameterized RPC calls that return multiple values over time. The protocol does not fit the use case.

Browser clients SHOULD use `application/duh-stream+json` via `fetch()`. The frame parsing required is minimal and keeps the mental model consistent across browser and non-browser clients.

---

### Bidirectional Streams

Bidirectional streaming (where both client and server send frames over the same connection) is not yet defined.

TODO: Define bidirectional streaming semantics. client-to-server frame structure, and half-close behavior.
