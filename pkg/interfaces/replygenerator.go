package interfaces

import (
	"context"
	"net/mail"

	gomail "github.com/wneessen/go-mail"
)

// ReplyGenerator generates reply body message based on input message
// Return nil if reply should not be sent
type ReplyGenerator interface {
	GenerateReplyBody(ctx context.Context, msg *mail.Message, mailfrom, rcptto string, reply *gomail.Msg) (*gomail.Msg, error)
}
