# Why not gRPC?

### gRPC wasn't built for the browser

A browser can't make a native gRPC call. The HTTP/2 trailers gRPC depends on aren't exposed to
browser JavaScript, so a web page reaches a gRPC backend through gRPC-Web, a separate protocol that a
proxy translates to gRPC in front of the service. That's an extra moving part to deploy, secure, and
reason about, and it's one a plain HTTP API never needs.

* [gRPC weaknesses (Microsoft)](https://learn.microsoft.com/en-us/aspnet/core/grpc/comparison?view=aspnetcore-8.0#grpc-weaknesses)
* [When to avoid gRPC (Red Hat)](https://www.redhat.com/architect/when-to-avoid-grpc)
* [gRPC-Web requires a proxy (Envoy)](https://blog.envoyproxy.io/envoy-and-grpc-web-a-fresh-new-alternative-to-rest-6504ce7eb880)
* [gRPC load balancing (Microsoft)](https://learn.microsoft.com/en-us/aspnet/core/grpc/performance?view=aspnetcore-8.0#load-balancing)

### One transport for internal and external

Because gRPC is awkward at the public edge, the moment you expose an API to customers or partner
vendors you reach for HTTP anyway. Adopting gRPC for internal traffic therefore means committing to
two transports at once, gRPC between your own services and HTTP at the edge. That's two client stacks
to build, two sets of tooling to learn, and two mental models your team carries.

Standardize on HTTP and the same transport serves both. One stack, internal and external calls alike.

### The framework is a commitment

gRPC is a framework you adopt, not a protocol you call. Every language needs a generated client
before it can talk to your service, where an HTTP client ships with the standard library everywhere.
Channels carry a connection lifecycle of their own, connect, close, and graceful shutdown, and fine
control over a call takes more boilerplate than you would expect.

This adds up. The people behind dRPC reached the same conclusion and
[rebuilt a lighter replacement](https://www.storj.io/blog/introducing-drpc-our-replacement-for-grpc)
because gRPC carried more than they needed. When we moved Gubernator from gRPC to plain HTTP we
removed around 2,000 lines of code, some of it written only to manage the connection lifecycle and
graceful shutdown.

### The framework decides before you do

Some requirements don't fit the shape gRPC chose for you, and you find out only when one arrives. Try
to upload a multi-megabyte file. You can marshal it into a single protobuf message and fight the
message-size limits and the memory it takes to hold the whole thing, or you can reach for gRPC
streaming and take on a second programming model just to move some bytes. Either way the framework
decided your options before the requirement existed.

Plain HTTP already carries opaque bytes. DUH-RPC keeps structured calls in protobuf or JSON and lets
unstructured data ride as a content endpoint, where uploading a file is just a POST of the file. The
transport never forces the shape of your data, so a requirement you didn't foresee doesn't cost you a
second protocol.

### Apples to apples

Compare like for like, the same serialization on both sides, and the framework shows up in the
numbers. gRPC adds overhead that a plain HTTP request carrying the same protobuf does not. Compare
JSON over REST instead and REST is the slower one, but that's the serialization talking, not the
transport.

The overhead isn't always where you'd look for it. We hit a wall on streaming once, and the cause
turned out to be that gRPC queues calls once they reach the concurrent-stream limit on a connection,
documented in the [gRPC performance guide](https://grpc.io/docs/guides/performance/). We had to
redesign around the limit once we understood it.

See the [DUH benchmarks](https://github.com/duh-rpc/duh-benchmarks.go) for a gRPC versus DUH
comparison in Go.

### You can use Protobuf without gRPC

This is the part people conflate most often. Protobuf is a serialization format, and it stands on its
own. You can and should use it without adopting gRPC.

That's exactly what DUH-RPC does. It sends protobuf over plain HTTP, which makes it read as a
constrained subset of REST in an RPC style. You keep the schema, the code generation, and the
performance, and any HTTP client can still call your service.
