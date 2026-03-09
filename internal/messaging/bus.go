package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

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
	return b.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		MaxLen: 1000000,
		Approx: true,
		Values: fields,
	}).Err()
}

func (b *Bus) Consume(ctx context.Context, stream, group, consumer string, handler func(redis.XMessage) error) error {
	err := b.client.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("messaging.Consume: create group: %w", err)
	}

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
				if err := handler(msg); err != nil {
					slog.Error("message handler failed", "stream", stream, "msg_id", msg.ID, "err", err)
					continue
				}
				b.client.XAck(ctx, stream, group, msg.ID)
			}
		}
	}
}

func (b *Bus) Close() error {
	return b.client.Close()
}
