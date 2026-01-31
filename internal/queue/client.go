package queue

import "context"

// Client sends messages to a queue backend.
type Client interface {
	Send(ctx context.Context, msg Message) error
}
