package postfixadminvacation

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"

	"github.com/rkojedzinszky/go-vacationd/pkg/interfaces"

	gomail "github.com/wneessen/go-mail"
)

type replygenerator struct {
	db *sql.DB
}

func New(db *sql.DB) (interfaces.ReplyGenerator, error) {
	return &replygenerator{
		db: db,
	}, nil
}

// GenerateReplyBody implements interfaces.ReplyGenerator.
func (r *replygenerator) GenerateReplyBody(ctx context.Context, msg *mail.Message, mailfrom string, rcptto string, reply *gomail.Msg) (*gomail.Msg, error) {
	var subject, body string

	if err := r.db.QueryRowContext(ctx, "SELECT subject, body FROM vacation WHERE active AND email = $1 AND NOW() BETWEEN activefrom AND activeuntil", rcptto).Scan(&subject, &body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, err
	}

	if subject != "" {
		reply.Subject(subject)
	}

	if body != "" {
		reply.SetBodyString(gomail.TypeTextPlain, body)
	}

	return reply, nil
}
