# Kernel API — mailbox full and client retry

## ErrMailboxFull

When a client sends a message to an actor via the kernel (e.g. gRPC `SendMessage`), the kernel enqueues the message to the actor’s mailbox. Each actor has a bounded mailbox (default capacity 1024). If the mailbox is full, the kernel does not enqueue and returns an error to the caller.

- **gRPC:** The kernel service typically returns a status with code **ResourceExhausted** or **Unavailable** and a message indicating the actor’s mailbox is full (e.g. `"mailbox full"` or `kernel.Send: ... ErrMailboxFull`).
- **Semantics:** The request was not applied. The client may retry the same message later.

## Client retry guidance

- **Back off:** When the kernel returns a mailbox-full (or equivalent) error, clients should **back off** and retry. Do not retry immediately in a tight loop.
- **Recommended:** Use exponential backoff (e.g. 1s, 2s, 4s) with a maximum delay (e.g. 30s) and a maximum number of retries. Optionally respect a `Retry-After` response header or trailing metadata if the kernel provides one (see below).
- **Idempotency:** If the operation is not idempotent, the client should use a stable idempotency key (e.g. for goal creation) so that a retry does not create duplicates.

## Optional: Retry-After metadata

The kernel (or API gateway) may set gRPC trailing metadata or an HTTP header `Retry-After` (seconds) when returning a mailbox-full error, so that clients can wait the suggested duration before retrying. This is optional and may be added in a future release.
