package interfaces

import (
	"context"
)

// RateLimiter limits rate of sending mails from -> to
type RateLimiter interface {
	// Ratelimit will return true when sending is permitted
	Ratelimit(from, to string) bool

	// Run will add/expire entries
	Run(context.Context)
}
