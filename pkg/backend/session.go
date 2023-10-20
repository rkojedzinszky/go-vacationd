package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"regexp"
	"time"

	"github.com/emersion/go-smtp"
)

var (
	fromRe = regexp.MustCompile("^[^@]+@[^@]+$")
	rcptRe = regexp.MustCompile("^([^#]+)#([^@]+)@([^@]+)$")

	errInvalidFrom       = errors.New("invalid from address")
	errInvalidRcpt       = errors.New("invalid rcpt address")
	errInvalidRcptDomain = errors.New("invalid rcpt domain")
)

type session struct {
	backend *backend
	c       *smtp.Conn

	from     string
	rcptreal map[string]string
	msg      *mail.Message
}

// AuthPlain implements smtp.Session.
func (*session) AuthPlain(username string, password string) error {
	return nil
}

// Logout implements smtp.Session.
func (*session) Logout() error {
	return nil
}

// Reset implements smtp.Session.
func (*session) Reset() {
}

// Set return path for currently processed message.
func (s *session) Mail(from string, opts *smtp.MailOptions) error {
	if from != "" && !fromRe.MatchString(from) {
		return errInvalidFrom
	}

	s.from = from

	return nil
}

// Add recipient for currently processed message.
func (s *session) Rcpt(to string, opts *smtp.RcptOptions) error {
	m := rcptRe.FindStringSubmatch(to)
	if m == nil {
		return errInvalidRcpt
	}

	if m[3] != s.backend.domain {
		return errInvalidRcptDomain
	}

	s.rcptreal[to] = fmt.Sprintf("%s@%s", m[1], m[2])

	return nil
}

// Data implements smtp.Session.
func (s *session) Data(r io.Reader) (err error) {
	return errors.New("unimplemented")
}

// Data implements smtp.LMTPSession.
func (s *session) LMTPData(r io.Reader, status smtp.StatusCollector) (err error) {
	s.msg, err = mail.ReadMessage(r)

	if err != nil {
		return
	}

	// discard body
	if _, err = io.Copy(io.Discard, s.msg.Body); err != nil {
		return
	}

	// ensure 15 sec processing time
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = s.backend.handleMail(ctx, s.msg, s.from, s.rcptreal, status)

	return
}

var _ smtp.Session = &session{}
var _ smtp.LMTPSession = &session{}
