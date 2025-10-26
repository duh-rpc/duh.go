I always struggle to write such articles, as I don't really like bashing other techs, so I'll have to come back to this
later. However...

### REST performance will always be slower than RPC
The main reason REST will always be slower than GRPC and DUH-RPC is due to REST requiring an HTTP Router for
matching complex REST paths like `/v1/{thing}/collection/{id}/foo`. Both GRPC and DUH-RPC use simple string matching to
connect an RPC method to a handler which results in unparalleled performance.

### The hierarchical nature of REST does not lend itself to clean interfaces over time.
RPC style APIs do not suffer from the hierarchical structure problems that rest APIs do.
(TODO: Write a blog post about this and link it here) or if you are at mailgun, read the 'HTTP and GRPC Design Guide'
in the Mailgun wiki.

### Rest calls typically become method calls in code
Given the notorious pet store example, https://petstore.swagger.io/ clients will typically and inevitably convert
those REST calls to RPC style method calls.

`GET /pet/findByStatus` becomes `find_pets_by_tags()`
`POST /pet` becomes `add_pet()`
`PUT /pet` becomes `update_pet()`
`DELETE /pet/{petId]` becomes `delete_pet()`

For machine-to-machine communication, REST offers few benefits over DUH-RPC.

### REST is stateless and thus cacheable
This is a semantic that RPC calls should emulate where is makes sense. CRUD operations are a great example of 
an appropriate use of a stateless API.

### Intermediaries
Intermediaries can exist between the client and the server. While this is often less useful due to the advent of TLS 
everywhere, it is still true of RPC methods. Most of the ways in which REST requests can be augmented and mutated is
via HTTP Headers which are still accessable and should still be utilized when designing and using RPC style API's. 
The best example of this is using for authZ and authN.

### REST has no standardized response body
Most REST APIs contrive their own response bodies depending on the experience of the designer and the use case involved.
This divergence of REST APIs makes it difficult for libraries and frameworks to handle errors uniformly.

However, n effort to standardize REST API error handling is underway, the IETF devised RFC 7807, which creates a 
generalized error-handling schema.

* type – a URI identifier that categorizes the error
* title – a brief, human-readable message about the error
* status – the HTTP response code (optional)
* detail – a human-readable explanation of the error
* instance – a URI that identifies the specific occurrence of the error

DUH-RPC takes some of its inspiration from this RFC and from GRPC to implement the `Reply` structure. With the 
most notable difference being the `detail` as a map of strings which is similar to HTTP headers.
