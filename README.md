Overview:

```
┌─────────────┐          ┌──────────────────┐          ┌─────────────┐
│   Client    │  HTTP    │ Payment Service  │   gRPC   │Rate Limiter │
│             ├─────────>│                  ├─────────>│             │
└─────────────┘          │  - Process       │          │  - Check    │
                         │  - Validate      │          │  - Register │
                         └──────────────────┘          └──────┬──────┘
                                                              │
                                                              v
                                                        ┌──────────┐
                                                        │  Redis   │
                                                        │  State   │
                                                        └──────────┘
```
The limiter supports to operations,

- `Register`: Services register their rate limit configs
- `Check`: Validate if request is allowed

The payment services registers itself on startup with rate limit rules and makes gRPC calls to rate limiter before processing requests 500ms timeout per check.

The HTTP client preserves as an end user simulation. It calls payment service HTTP endpoints and validates rate limits are enforced.

Using this architecture makes it easy to do horizontal scaling by running multiple rate limiter instances and all will share the same Redis cluster.

A run script that initializes the Redis instance and starts the client test is included (./run.sh). A Dockerized version is also included if you don't have Redis installed, you can simply run `docker-compose run --rm test-client`.
