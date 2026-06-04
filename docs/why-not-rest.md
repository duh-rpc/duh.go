# Why not REST?
REST is a style, not a specification. It tells you to model your API as resources and lean on HTTP's verbs, then it leaves the rest to you. Where do the parameters go? What does an error look like? How do you page through a list?

REST hands you a decision on every endpoint; RPC takes it away. What's left is an API that looks the same everywhere, because there was only ever one way to write it. The rest of this is the long version of that sentence, including the parts REST genuinely gets right.

### REST makes you decide the same thing on every endpoint

Pick any REST API that's been alive a few years and look at how it takes input. You'll find an id in a path on one endpoint, in a query string on the next, and in a JSON body on the third. Not because anyone was careless, but because all three are legal REST, and whoever wrote each one picked what felt right that day.

Now multiply that by every decision REST leaves open. Which verb does an action that isn't quite CRUD use, PUT or POST? Is a not-found a 404 with an empty body, a 404 with `{"error": "..."}`, or a 200 with `{"found": false}`? Does a list page with `?offset=`, `?page=`, a cursor, or a `Link` header? Each question has several correct answers, so different parts of the same API answer them differently.

None of this is a bug. It's the accumulated cost of a style that prizes flexibility over agreement. Every "do it however you like" is something your team now has to decide, document, and remember, on every endpoint, forever.

You can fix this by writing the conventions down, putting them in an OpenAPI spec, and running a linter in CI that rejects any endpoint that strays from your guidelines. Plenty of teams do exactly this, and it works; the divergence stops.

### Some of those choices are one-way doors

Let's say your spec standardizes on query parameters and form data for all input. It's clean, it's consistent, and your linter enforces it on every endpoint.

Then we need an API to send a deeply nested object, the kind of structure query strings and form encoding were never built to carry. Now you have two bad options. Bolt a JSON body onto that one endpoint and break the convention you spent a year enforcing, or contort the data into a flat shape it doesn't want. The early choice was a one-way door, and you didn't notice until you needed to walk back through it.

gRPC isn't immune to this; it just has different doors. Try to upload a multi-megabyte file. You can marshal it into a single protobuf message and fight the size limits and the memory, or you can reach for gRPC streaming and take on a whole second programming model to move some bytes. The framework decided your options before the requirement existed.

DUH-RPC doesn't have this failure mode, because it never takes the choice away from you. Structured endpoints carry messages, nested or not, as JSON or protobuf in the body. Content endpoints carry opaque bytes; if you need to upload a file, you POST the file, exactly as the spec describes for unstructured data. Plain HTTP already carries both, so the transport never forces the shape of your data.

That's the real freedom REST and gRPC were each reaching for, and each gave up somewhere. You should never have to design around the framework you chose.

### The hierarchy you pick today is wrong tomorrow

Another issue with REST APIs is that they encode one view of the world, the hierarchy you happened to have the day you designed it.

At Mailgun we built contact lists nested under the domain that owned them, `/v1/accounts/{id}/domains/{domain}/contacts`. It was the obvious shape; a contact list belonged to a domain. Months later product wanted account-wide contact lists, and the obvious shape for those was `/v1/accounts/{id}/contacts`. Now the word "contacts" meant two different things depending on where you stood in the hierarchy, and neither path could be retired.

With a public API the old hierarchy never really goes away, so a "fix" means a second hierarchy standing next to the first, bugs fixed twice and both tested forever.

But wait, REST has an escape for this, and I've recommended it myself. Flatten the hierarchy and push the variable part into a query string, `/v1/contacts?domain=example.com`, so that adding an `account_id` later is a new parameter instead of a re-shaped path. It genuinely helps. But a query string is the dead end from the last section wearing a different hat, and your input is now scattered between the path and the query besides.

See [The RESTful Hierarchy Problem](https://wippler.dev/posts/The-Restful-Hierarchy-Problem).

### The path is the wrong place for data

The path is for routing. The moment you put a user-controlled value in it, you've turned a routing problem into a parsing problem, and you don't own all the parsers.

At Mailgun we had `/v2/domains/{domain}/tags/{tag}`. It worked until someone created a tag named `#trending`. A `#` is legal in a URL, where it marks the start of a fragment, so Flask read `#trending` as an anchor and routed the request as if the tag weren't there at all. The tag was valid. The endpoint was valid. The URL grammar disagreed with both.

The deeper problem is that a request crosses a JavaScript frontend, a proxy, and a backend, three different URL parsers written against the same RFC by people who read it differently. As long as user data sits in the path, you're betting all three agree on every character a customer can type. They don't, and the failures are the worst kind, silent misroutes and security holes rather than clean errors.

You can adapt by normalizing the value and swap the awkward characters, until `2020/02/01` and `2020_02_01` collapse into the same path and your users can't tell two resources apart. Or trade the name for an opaque id and add a second API just to look the id up. Every fix buys back one corner by selling another.

DUH-RPC keeps the path to a fixed `subject.method` that you control end to end, never the customer. Every user-controlled value rides in the body, where a single encoder, JSON or protobuf, handles `#trending` and `2020/02/01` and anything else identically on every hop. The router never sees customer data, so it never has to have an opinion about it.

See [Mastering RESTful Design](https://wippler.dev/posts/Mastering-RESTful-Design).

### REST endpoints become method calls anyway

Open the canonical Swagger pet store and generate a client from its OpenAPI spec. You don't get a tour of resources and verbs; you get a list of methods. `add_pet()`, `find_pets_by_status()`, `update_pet()`, `delete_pet()`. The paths and the HTTP verbs are gone, compiled away into function names.

That isn't the generator being lazy. It's the generator admitting what the API actually is. Nobody writes `http.PUT("/pet")` in application code; they call `update_pet()`, because the operation is the unit a developer thinks in, not the resource and not the verb.

The server side admits it too. The operations that were never CRUD (cancel an order, retry a job, send a message) come out as verb sub-resources, `POST /orders/{id}/cancel`, `POST /jobs/{id}/retry`. Those are RPC calls wearing a REST costume. Both ends of the wire quietly agree the real interface is a set of methods, then spend effort translating to and from a resource model in between.

So REST became a layer in the middle that both sides paid for and neither used. The client generated its way out of it; the server bent its operations around it.

DUH-RPC skips the translation. `/v1/pets.add`, `/v1/pets.findByStatus`, `/v1/orders.cancel`. The wire format and the method call are the same shape, so there's nothing to generate, nothing to keep in sync, and nothing to get wrong between them. For machine-to-machine traffic, that middle layer was the only thing REST was adding, and removing it costs you nothing.

### A router is slower than a switch

Every REST request has to be routed, and routing a REST path is pattern matching. The router holds a set of templates like `/v1/{thing}/collection/{id}/foo`, and for each request it walks them, pulls the variables out of the path, and works out which one matched. The more endpoints you have and the more variables each path carries, the more work that is.

RPC dispatch isn't pattern matching. The path is a fixed string, `/v1/users.create`, and the server looks it up in a map. No variables to extract, no templates to walk, just an exact match to a handler. gRPC works this way and so does DUH-RPC, and it's the cheaper operation by construction.

But be careful what you credit the difference to, because two separate costs usually get blended into one. People say REST is slow and mean JSON; they say gRPC is fast and mean protobuf. Serialization and routing are different things, and most "REST vs gRPC" benchmarks quietly change both at once.

Pull them apart. The router-versus-switch gap is real but small. The JSON-versus-protobuf gap is the bigger lever, and it has nothing to do with REST; it's a serialization choice you can make on either side. DUH-RPC lets you keep protobuf without taking on a framework to get it, which is the whole point. You don't need gRPC to stop paying the JSON tax.

And be honest about the stakes. For most services neither gap is the bottleneck; your database is. Don't choose an API style for the microseconds. But when you genuinely need the headroom, the switch and protobuf are both already there, and you didn't adopt a framework to reach them. The [DUH benchmarks](https://github.com/duh-rpc/duh-benchmarks.go) compare like for like if you want the numbers.

### What REST gets right, and RPC keeps

None of this means REST was a mistake. It got real things right, and a good RPC style keeps them instead of throwing them out with the resource hierarchy.

Statelessness is the big one. A REST request carries everything the server needs to handle it, with no session sitting on the server between calls, and that property is most of what lets REST scale horizontally. DUH keeps it. Every call is self-contained and there's no connection-level state to coordinate.

Caching is the part people get wrong because they file it under REST when it belongs to content delivery. Transparent HTTP caching is a content-delivery property, not an API property. It rides on GET and URL cache keys, and it's how CDNs and browsers serve the images and static assets a UI loads over and over. Your service-to-service calls are dynamic and usually authenticated; you do not want a proxy handing back a stale one, so POST-only costs your API nothing it ever wanted. Where caching actually helps an API, you do it deliberately at the application layer, where you control what's safe to reuse. And where you're shipping cacheable content to a browser, that's a plain GET behind a CDN, which DUH never stops you running alongside your API.

Intermediaries are the second. Proxies, gateways, and load balancers can act on a request without understanding its body, as long as the signals they need live in headers. That's still true under DUH. Authentication and authorization go in headers the way they always did, and DUH adds its own (`X-DUH-Version`, the `X-RPC-` metadata convention) in the same spirit. The body is yours; the headers are the network's.

And the error shape. REST's original sin is that it never had a standard one, the divergence from the first section at its worst; every API invents its own error body and no library can handle them all. But the REST community saw the problem and answered it with RFC 7807, a single `problem+json` schema with `type`, `title`, `status`, `detail`, and `instance`. That instinct is exactly right. DUH's `Reply` takes its inspiration from RFC 7807 and from gRPC's status, with the main difference that `details` is a map of strings, closer to how headers already work.

The goal was never to reject REST wholesale. It was to keep the parts that earned their place (stateless requests, header-driven intermediaries, one error shape) and drop the part that costs the most, the resource hierarchy you have to design around forever.

### RPC has only one shape to offer

REST gives you a hundred good choices and asks you to make every one, on every endpoint, then live with them. Each choice is defensible; the sum is an API that diverges by taste, locks you in by accident, and freezes an org chart you've already outgrown.

RPC makes the choices once. Input in the body, POST, a cursor, a `Reply`, a path that names the method. Consistency isn't something you have to enforce here; the style only has one shape to offer, and nothing left to design around.
