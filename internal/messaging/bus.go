package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultMaxRetries = 3
	defaultMinIdle    = 30 * time.Second
	retryKeyPrefix    = "astra:retry:"
	retryKeyTTL       = time.Hour
	consumerDeadLetterStream = "astra:dead_letter"
)

// ConsumeOptions configures retry and reclaim behavior. Zero value uses defaults.
type ConsumeOptions struct {
	MaxRetries int
	MinIdle    time.Duration
}

type Bus struct {
	client *redis.Client
}

func New(addr string) *Bus {
	return &Bus{
		client: redis.NewClient(&redis.Options{
			Addr:         addr,
			DialTimeout:  2 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			PoolSize:     50,
		}),
	}
}

func (b *Bus) Publish(ctx context.Context, stream string, fields map[string]interface{}) error {
	_, err := b.PublishReturnID(ctx, stream, fields)
	return err
}

func (b *Bus) PublishReturnID(ctx context.Context, stream string, fields map[string]interface{}) (string, error) {
	return b.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		MaxLen: 1000000,
		Approx: true,
		Values: fields,
	}).Result()
}

func (b *Bus) Consume(ctx context.Context, stream, group, consumer string, handler func(redis.XMessage) error) error {
	return b.ConsumeWithOptions(ctx, stream, group, consumer, handler, ConsumeOptions{})
}

func (b *Bus) ConsumeWithOptions(ctx context.Context, stream, group, consumer string, handler func(redis.XMessage) error, opts ConsumeOptions) error {
	err := b.client.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("messaging.Consume: create group: %w", err)
	}
	maxRetries := opts.MaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}
	minIdle := opts.MinIdle
	if minIdle <= 0 {
		minIdle = defaultMinIdle
	}
	go b.runAutoClaimWith(ctx, stream, group, consumer, maxRetries, minIdle, handler)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msgs, err := b.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: consumer,
			Streams:  []string{stream, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()
		if err != nil {
			slog.Warn("stream read error, retrying", "stream", stream, "err", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		for _, s := range msgs {
			for _, msg := range s.Messages {
				b.processMessageWith(ctx, stream, group, consumer, maxRetries, msg, handler)
			}
		}
	}
}

func (b *Bus) retryKey(stream, group, msgID string) string {
	return retryKeyPrefix + stream + ":" + group + ":" + msgID
}

func (b *Bus) getAndIncrementRetryCount(ctx context.Context, stream, group, msgID string) (int, error) {
	key := b.retryKey(stream, group, msgID)
	pipe := b.client.Pipeline()
	incr := pipe.HIncrBy(ctx, key, "count", 1)
	pipe.Expire(ctx, key, retryKeyTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return int(incr.Val()), nil
}

func (b *Bus) publishConsumerDeadLetter(ctx context.Context, stream, group, msgID string, retryCount int, lastError string, msg redis.XMessage) {
	taskID := ""
	if v, ok := msg.Values["task_id"]; ok {
		if s, ok := v.(string); ok {
			taskID = s
		}
	}
	payload := map[string]interface{}{
		"stream":       stream,
		"group":        group,
		"message_id":   msgID,
		"retry_count":  retryCount,
		"last_error":   lastError,
		"timestamp":    time.Now().Unix(),
	}
	if taskID != "" {
		payload["task_id"] = taskID
	}
	if err := b.Publish(ctx, consumerDeadLetterStream, payload); err != nil {
		slog.Warn("publish to consumer dead_letter failed", "stream", stream, "msg_id", msgID, "err", err)
	}
}

func (b *Bus) processMessageWith(ctx context.Context, stream, group, consumer string, maxRetries int, msg redis.XMessage, handler func(redis.XMessage) error) {
	err := handler(msg)
	if err == nil {
		b.client.XAck(ctx, stream, group, msg.ID)
		b.client.Del(ctx, b.retryKey(stream, group, msg.ID))
		return
	}
	slog.Error("message handler failed", "stream", stream, "msg_id", msg.ID, "err", err)

	count, incrErr := b.getAndIncrementRetryCount(ctx, stream, group, msg.ID)
	if incrErr != nil {
		slog.Warn("retry count increment failed", "stream", stream, "msg_id", msg.ID, "err", incrErr)
		return
	}
	if count >= maxRetries {
		b.publishConsumerDeadLetter(ctx, stream, group, msg.ID, count, err.Error(), msg)
		b.client.XAck(ctx, stream, group, msg.ID)
		b.client.Del(ctx, b.retryKey(stream, group, msg.ID))
	}
}

func (b *Bus) runAutoClaimWith(ctx context.Context, stream, group, consumer string, maxRetries int, minIdle time.Duration, handler func(redis.XMessage) error) {
	ticker := time.NewTicker(minIdle)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			start := "0-0"
			for {
				claimed, next, err := b.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
					Stream:   stream,
					Group:   group,
					Consumer: consumer,
					MinIdle:  minIdle,
					Start:    start,
					Count:    10,
				}).Result()
				if err != nil {
					slog.Warn("xautoclaim failed", "stream", stream, "err", err)
					break
				}
				for _, msg := range claimed {
					b.processMessageWith(ctx, stream, group, consumer, maxRetries, msg, handler)
				}
				if next == "0-0" {
					break
				}
				start = next
			}
		}
	}
}

func (b *Bus) Close() error {
	return b.client.Close()
}

// GetPendingCount returns the number of pending messages for the consumer group (for metrics).
func (b *Bus) GetPendingCount(ctx context.Context, stream, group string) (int64, error) {
	info, err := b.client.XInfoGroups(ctx, stream).Result()
	if err != nil {
		return 0, err
	}
	for _, g := range info {
		if g.Name == group {
			return g.Pending, nil
		}
	}
	return 0, nil
}
