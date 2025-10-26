## Why not GPRC?
While GRPC has several advantages, there are a few aspects that nudged us toward a using standard HTTP. Firstly, it
demands a language-specific framework, with service definitions in Protobuf. This can deter broad adoption, especially
for software as a service-based companies who wish to have customers use their APIs. Also, large interdepartmental
or business units have to create and provide APIs for integration and operation of their respective units. A strict
GRPC interface ecosystem is only viable within an organization which is completely committed to its adoption, and
has accepted that there will be one-off standard HTTP endpoints for external customers.

### GRPC is not suitable for the Web
As mentioned above, when adopting GRPC you accept that any public facing API's designed to be accessible via the WEB
at scale cannot be GRPC. GRPC is mostly useful as an internal communication method to make calls between services.

Much as been written about why GRPC is not suitable for the web, but the core rational is that GRPC uses sticky
connections, and it prefers client side load balancing. Both of which make load balancing clients difficult by
placing control in the hands of the client, not the API service provider.

* [GRPC Weaknesses via Microsoft](https://learn.microsoft.com/en-us/aspnet/core/grpc/comparison?view=aspnetcore-8.0#grpc-weaknesses)
* [When to avoid GRPC via Redhat](https://www.redhat.com/architect/when-to-avoid-grpc)
* [GRPC-Web requires a proxy](https://blog.envoyproxy.io/envoy-and-grpc-web-a-fresh-new-alternative-to-rest-6504ce7eb880)
* [GRPC Load Balancing via Microsoft](https://learn.microsoft.com/en-us/aspnet/core/grpc/performance?view=aspnetcore-8.0#load-balancing)

### GRPC is more complex than is necessary for high-performance, distributed environments.
As you dive deeper into GRPC, you'll notice GRPC carries some baggage in the form of
[deprecated systems and a somewhat bloated structure](https://www.storj.io/blog/introducing-drpc-our-replacement-for-grpc),
which can leave users scratching their heads on what's best to use and how. The boiler plate code needed for fine
control over GRPC compared to native HTTP can be much higher than you might expect as unlike most HTTP clients you
have connection semantics, like connect and close. When switching Gubernator from GPRC to HTTP, we were able to 
remove a total of 2,000 lines of code, some of which was written just to handle complex client shutdown.

### GRPC implementations can be slower than standard HTTP
When performance suffers, it's not always clear why GPRC is doing what it's doing. For instance, we ran into a strange
performance issue when using streaming which led us to discover that GRPC queue's requests once the concurrent number 
of streams has been hit. (Our only recourse was to re-design the system with the knowledge of GPRCs nonsensical limitations.)
(See [GRPC Docs](https://grpc.io/docs/guides/performance/))

With the proliferation of HTTP2, the gap of performance between GRPC and HTTP1 has been greatly and in many cases
eclipsed by standard HTTP/2 implementation. Our own hands-on experiments showed that GRPC's heralded performance is
not as great as is assumed to be, especially in high-concurrency, low-latency scenarios. In fact, the entire reason this
spec exists is the realization, that our standard HTTP services were outperforming our GRPC based services.

See https://github.com/duh-rpc/duh-benchmarks.go for a comparison of DUH with GRPC in golang.

### GRPC and Service Mesh
We used a service mesh at Mailgun that was incompatible with GRPC, so we had to use the client side load balancing and
consul discovery via DNS. This worked, but was a one off in our normal environment, when diagnosing issues we had to 
constantly remind ourselves that GRPC was different than how all our other services operated, and didn't show up on 
any of our standard HTTP tooling. This is avoidable by using a GRPC compatible mesh, but is just one more cognitive
load to add to your already complex system.

### GRPC Gateway
We naively thought that GRPC gateway would solve our compatibility and ..... (finish thought)
TODO: Talk about the performance issues we have experienced using GRPC gateway.
TODO: TLS connection to local gateway issue. (See Gubernator)

### GPRC is not Protobuf
You can and should use Protobuf. You don't need to use GRPC to use Protobuf.
