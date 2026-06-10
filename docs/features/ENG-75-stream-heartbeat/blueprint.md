# Stream Heartbeat Blueprint

## Objective

DUH-RPC structured streams have no mechanism to keep idle connections alive through intermediaries, detect half-open TCP connections, or let clients distinguish a dead server from a slow one. Today, all three failures surface as `io.ErrUnexpectedEOF` — indistinguishable from a server crash mid-frame.

This feature adds a heartbeat frame to the streaming wire protocol. The server sends heartbeats automatically; the client silently consumes them and optionally detects their absence as a dead-connection signal.

## Mental Model

A heartbeat is an empty frame the framework sends on behalf of the handler when the stream is idle. The handler never sees it. The client never sees it. It exists purely to keep the bytes flowing so that infrastructure between the two endpoints — proxies, load balancers, firewalls — does not kill the connection for being idle, and so the client can detect when the server has truly gone away.

The handler's view of the world does not change: call `Send` when you have data, call `Close` when you're done. The framework handles the rest.

## Correctness Constraints

### State Invariants

- **Frame ordering**: heartbeat frames never interleave with a partially-written data, final, or error frame. A frame is atomic on the wire — the mutex guarantees this.
- **Post-close silence**: no heartbeat frame is ever written after a final or error frame. The heartbeat goroutine stops before `Close` returns.
- **Single writer at a time**: despite two goroutines (handler + heartbeat ticker), only one holds the mutex and writes to the underlying `io.Writer` at any moment.

### Behavioral Constraints

- **Never surface heartbeats to callers**: `StreamReader.Recv` never returns a heartbeat frame to the caller. It consumes them internally and loops to read the next frame.
- **Never block Send on heartbeat**: acquiring the mutex for a heartbeat write must not delay a `Send` by more than the time to write 5 bytes + flush. Heartbeat frames are zero-payload; the critical section is tiny.
- **Heartbeat timeout is a distinct error**: when the client detects a heartbeat timeout, it returns a distinguishable error — not `io.ErrUnexpectedEOF`, not `io.EOF`. Callers can programmatically tell "the connection went silent" apart from "the server crashed mid-frame" or "the stream ended normally."

## Acceptance Criteria

- A stream that sends no data for 60 seconds survives through a proxy with a 60-second idle timeout (heartbeats keep it alive at 30-second intervals).
- A client connected to a server that silently dies receives a heartbeat timeout error within 60 seconds (2x heartbeat interval), not minutes later via TCP timeout.
- Existing handler *logic* (no `StreamConfig`, no awareness of heartbeats) works unchanged — heartbeats are sent automatically with defaults. Call sites must add `nil` as the fourth argument to `HandleStream`.
- `HandleStream` with `&StreamConfig{HeartbeatInterval: -1}` sends no heartbeat frames.
- Client with `HeartbeatTimeout: -1` never times out on missing heartbeats.
- The `Recv` loop in every existing client works unchanged — heartbeat frames are invisible.

## Scope

### In Scope

- `FlagHeartbeat` (`0x3`) wire format addition to the streaming spec
- Automatic server-side heartbeat sending (30s default, configurable via `StreamConfig`)
- Client-side silent consumption of heartbeat frames in `Recv`
- Client-side heartbeat timeout detection (60s default, configurable via `Client.HeartbeatTimeout`)
- New `ErrHeartbeatTimeout` error type distinguishable from `io.ErrUnexpectedEOF`
- Spec update to `docs/streaming.md` — match the existing spec's voice: direct, RFC-style (MUST/SHOULD/MAY), explains the *why* behind each rule, uses concrete wire-format examples with hex flag values and JSON payloads

### Out of Scope / Non-Goals

- Heartbeats for unstructured streams (`application/octet-stream`) — no framing layer to carry them
- Bidirectional heartbeats (client-to-server pings) — server-initiated only
- Automatic reconnection or retry on heartbeat timeout — the caller owns retry policy
- Heartbeats for bidirectional streaming — bidirectional is not yet defined in the spec

---

## Functional

### Wire Format

The existing frame header is unchanged:

```
[ 1 byte: flag ][ 4 bytes: uint32 big-endian length ][ N bytes: payload ]
```

New flag value:

| Flag | Value | Payload | Meaning |
|------|-------|---------|---------|
| `FlagData` | `0x0` | message bytes | Data frame |
| `FlagFinal` | `0x1` | optional message bytes | Clean end of stream |
| `FlagError` | `0x2` | `Reply` bytes | Error termination |
| `FlagHeartbeat` | `0x3` | empty (length = 0) | Keep-alive signal |

A heartbeat frame is always exactly 5 bytes on the wire: `[0x03][0x00][0x00][0x00][0x00]`.

Receivers MUST ignore the payload of a heartbeat frame (if any future version adds one). Receivers that encounter an unknown flag value SHOULD skip the frame (read and discard the payload bytes indicated by the length field) rather than treating it as an error — this is existing behavior since `stream.Reader.ReadFrame` returns the raw flag to the caller.

### Server Behavior

When `HandleStream` is called, the framework:

1. Starts a background goroutine with a `time.Ticker` at the configured heartbeat interval (default 30s).
2. The ticker goroutine acquires `streamWriter.mu`, writes a `FlagHeartbeat` frame with zero-length payload, flushes, and releases the lock.
3. When `Close` is called (or the handler returns an error), the framework signals the heartbeat goroutine to stop via a `stop` channel and waits for it to exit before writing the final/error frame.
4. If `HeartbeatInterval` is negative, no heartbeat goroutine is started.
5. If `HeartbeatInterval` is zero, the default (30s) is used.

### Client Behavior

When `DoStream` returns a `StreamReader`:

1. `Recv` reads frames in a loop. If it encounters `FlagHeartbeat`, it resets the heartbeat deadline and continues to the next frame — the caller never sees it.
2. If `HeartbeatTimeout` is positive (default 60s), the client sets a read deadline on each `Recv` call. If no frame (of any kind) arrives within the timeout, `Recv` returns `ErrHeartbeatTimeout`.
3. If `HeartbeatTimeout` is negative, no timeout is enforced.
4. If `HeartbeatTimeout` is zero, the default (60s) is used.
5. Every received frame (data, final, error, or heartbeat) resets the timeout clock.

### Error Type

```go
var ErrHeartbeatTimeout = errors.New("stream heartbeat timeout: no frames received within deadline")
```

This is a sentinel error. Callers can check it with `errors.Is(err, duh.ErrHeartbeatTimeout)`.

## Architecture

### Server-Side `streamWriter` Changes

The `streamWriter` struct gains three fields:

```go
type streamWriter struct {
	mu      sync.Mutex
	w       *stream.Writer
	flusher http.Flusher
	marshal marshalFunc
	closed  bool
	ctx     context.Context
	stop    chan struct{}
	stopped chan struct{}
}
```

- `mu` protects all writes to `w` and `flusher`, and guards the `closed` flag.
- `stop` is closed by `Close` (or the error path in `HandleStream`) to signal the heartbeat goroutine.
- `stopped` is closed by the heartbeat goroutine when it exits; `Close` waits on it to ensure no heartbeat races with the final/error frame.

`Send` and `Close` acquire `mu`. `Close` additionally closes `stop` and waits on `stopped` before writing the final frame under the lock.

### `HandleStream` Changes

```go
func HandleStream(w http.ResponseWriter, r *http.Request,
    handler func(*http.Request, StreamWriter) error, conf *StreamConfig)
```

The fourth parameter is `*StreamConfig`. `nil` means all defaults. This is a mechanical breaking change. All callers must add `nil` as the fourth argument; no handler body changes are required.

```go
type StreamConfig struct {
	HeartbeatInterval time.Duration
}
```

`HandleStream` resolves the interval (zero → 30s, negative → disabled), constructs `streamWriter`, starts the heartbeat goroutine (if enabled), calls the handler, then cleans up.

### Client-Side `streamReader` Changes

The `streamReader` struct gains a `heartbeatTimeout` field:

```go
type streamReader struct {
	r              *stream.Reader
	resp           *http.Response
	unmarshal      unmarshalFunc
	cancel         context.CancelFunc
	done           bool
	hasPending     bool
	heartbeatTimeout time.Duration
}
```

The `Recv` method changes:

1. Before reading, if `heartbeatTimeout > 0`, start a `time.AfterFunc` that calls `sr.cancel()` after the timeout duration. This is transport-agnostic — it works for both HTTP/1.1 and HTTP/2 without reaching into the underlying connection.
2. Read a frame. If the read succeeds, stop the timer. If `FlagHeartbeat`, loop back to step 1 (reset timer, read again).
3. If the timer fires and cancels the context, `ReadFrame` returns an error. `Recv` translates this to `ErrHeartbeatTimeout`.

### `Client` Struct Changes

```go
type Client struct {
	Client           *http.Client
	MaxFramePayload  int
	HeartbeatTimeout time.Duration
}
```

`DoStream` passes `HeartbeatTimeout` to the `streamReader` it constructs (zero → 60s, negative → disabled).

### `stream` Package — No Changes

`stream.Writer.WriteFrame` and `stream.Reader.ReadFrame` already handle arbitrary flag values. The `FlagHeartbeat` constant is added to `stream/stream.go` for convenience, but the reader/writer logic is unchanged.

## Data Design

No persistent data. All state is in-memory for the duration of a single stream.

### Invariant Preservation

| Invariant | Operations that touch it | Preservation mechanism |
|---|---|---|
| Frame ordering | `Send`, `Close`, heartbeat tick | `sync.Mutex` — only one goroutine writes at a time |
| Post-close silence | `Close`, heartbeat tick | `Close` closes `stop` channel and waits on `stopped` channel before writing final frame. Heartbeat goroutine checks `stop` in its select and exits. |
| Single writer | `Send`, `Close`, heartbeat tick | `sync.Mutex` on `streamWriter` |

### Illegal State Analysis

- **Heartbeat after close**: structurally prevented — `Close` waits for the heartbeat goroutine to exit (`<-stopped`) before writing the final frame. The goroutine cannot write after `stopped` is closed.
- **Concurrent frame writes**: structurally prevented by `sync.Mutex`. Two goroutines cannot hold the lock simultaneously.
- **Client receiving heartbeat**: prevented by application logic — `Recv` loops on heartbeat frames. This relies on correct code, not structural enforcement; the frame type check in `Recv` is the enforcement point.

## Testing

Testing follows the `surface-testing` skill.

### Key Surfaces

- **Integration (server + client over httptest)**: the primary surface. All heartbeat behavior is tested through the existing `httptest.NewServer` + `test.Client` pattern in `functional_test.go`.
- **Unit (stream package)**: round-trip tests for the new `FlagHeartbeat` constant — write a heartbeat frame, read it back, confirm flag and zero-length payload.

### Test Cases

**Server-side automatic heartbeats:**
- Stream with a slow handler (sleeps > heartbeat interval) — client receives data frames with heartbeat frames interspersed; client `Recv` only returns data frames.
- Stream with `HeartbeatInterval: -1` — no heartbeat frames sent.
- Stream with custom interval — heartbeat frames arrive at the configured cadence.

**Client-side heartbeat timeout:**
- Server sends one data frame then goes silent (no close, no heartbeat) — client `Recv` returns `ErrHeartbeatTimeout` after the timeout period.
- Server sends heartbeats but no data — client does not timeout (heartbeats reset the clock); client blocks in `Recv` until data or close arrives.
- Client with `HeartbeatTimeout: -1` — no timeout, blocks until TCP timeout or context cancel.

**Concurrency correctness:**
- Rapid `Send` calls interleaved with heartbeat ticks — no panics, no corrupted frames, no interleaved bytes. Verified by reading all frames on the client and confirming each is well-formed.

**Backward compatibility:**
- Existing tests pass unchanged (with `nil` config parameter added to `HandleStream` calls).

### Fakes / Substitutes

- **Slow handler**: `time.Sleep` inside a handler function to force heartbeat ticks between sends.
- **Silent server**: handler that sends one frame then blocks on a channel (simulating a hang), allowing the client timeout to fire. The channel is released in test cleanup.
- **Time**: real `time.Ticker` and `time.AfterFunc` — intervals are short in tests (e.g., 50ms heartbeat, 150ms timeout). No injectable clock needed; the intervals are configurable via `StreamConfig` and `Client.HeartbeatTimeout`.

## Limitations & Future Work
- **Bidirectional heartbeats**: if bidirectional streaming is added later, the client may also need to send heartbeats to the server. The `FlagHeartbeat` frame type is ready for this — the server reader would silently consume them the same way the client does.
- **Heartbeat interval negotiation**: the client currently has no way to know the server's heartbeat interval. The client timeout is configured independently. A future enhancement could include the interval in the response headers (e.g., `X-DUH-Heartbeat-Interval: 30`), but for now the 2x-interval default is a safe convention.

## Open Questions

None.
