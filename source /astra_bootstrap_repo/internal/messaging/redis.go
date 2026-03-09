
package messaging

import (
    "context"
    "github.com/redis/go-redis/v9"
)

type Bus struct {
    Client *redis.Client
}

func New(addr string) *Bus {
    r := redis.NewClient(&redis.Options{Addr: addr})
    return &Bus{Client: r}
}

func (b *Bus) Publish(stream string, payload map[string]interface{}) error {
    return b.Client.XAdd(context.Background(), &redis.XAddArgs{
        Stream: stream,
        Values: payload,
    }).Err()
}
