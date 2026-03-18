# Redis consumer retry and reclaim — spec

## Retry policy

- **Max retries (N):** 3. After 3 handler failures for the same message, the message is published to `astra:dead_letter` and the original message is XAcked (so it is not redelivered).
- **MinIdle for reclaim:** 30 seconds. Pending messages that have been idle (no XAck) for longer than MinIdle can be claimed by another consumer via XAutoClaim (or periodic XCLAIM) so that a stuck consumer does not hold messages forever.

## Retry count storage

Store per-message retry count in a **Redis hash** keyed by a single key per message: `astra:retry:{stream}:{group}:{msg_id}` with field `count` (integer). TTL e.g. 1 hour so keys do not leak. Alternative: store in a stream field on the message itself (would require re-adding with additional field on each claim, which Redis Streams does not support for existing messages). So Redis hash is used.

- On handler error: HINCRBY `astra:retry:{stream}:{group}:{msg_id}` count 1; EXPIRE key 3600.
- If count >= N: publish to astra:dead_letter (see below), then XAck the original message and DEL the retry key.
- On handler success: XAck the original message; DEL the retry key (cleanup).

## astra:dead_letter message shape

Messages published to `astra:dead_letter` (for consumer failures, not task failures) have the following fields:

- `stream` (string): original stream name, e.g. `astra:tasks:shard:0`
- `group` (string): consumer group name
- `message_id` (string): Redis message ID (e.g. `"1234-0"`)
- `retry_count` (int): number of handler failures before giving up
- `last_error` (string): error message from the last handler failure
- `task_id` (string, optional): if the original message contained `task_id`, include it for correlation
- `timestamp` (int): Unix seconds when the message was moved to dead letter

Callers (e.g. execution-worker) that consume from task streams already publish task-level dead_letter to `astra:dead_letter` with task_id, goal_id, error, timestamp. The Bus-level dead_letter (for handler errors) uses the shape above so that both task final failures and consumer handler failures can be distinguished and processed.

## Reclaim (XAutoClaim)

Run a ticker (e.g. every 30s) or integrate into the read loop: call `XAutoClaim(ctx, stream, group, consumer, MinIdle, "0-0")` to claim pending messages that have been idle longer than MinIdle. For each claimed message, re-invoke the handler. This allows another consumer (or the same one after restart) to process messages that were left pending by a crashed consumer. Use the same retry count logic: on handler error increment count; if count >= N, publish to dead_letter and XAck.
