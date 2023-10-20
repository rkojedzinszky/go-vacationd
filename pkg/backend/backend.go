package backend

import (
	"context"
	"fmt"
	"log"
	"net/mail"
	"regexp"
	"strings"

	"github.com/emersion/go-smtp"
	gomail "github.com/wneessen/go-mail"

	"github.com/rkojedzinszky/go-vacationd/pkg/interfaces"
)

const (
	referencesHeader                    = "References"
	autoSubmittedHeader                 = "Auto-Submitted"
	autoSubmittedHeaderNo               = "no"
	autoSubmittedHeaderAutoReplied      = "auto-replied"
	xAutoResponseSuppressHeader         = "X-Auto-Response-Suppress"
	xAutoResponseSuppressHeaderAllValue = "All"
	xAutoResponseSuppressHeaderOOFValue = "OOF"

	headerResentTo  = "Resent-To"
	headerResentCC  = "Resent-Cc"
	headerResentBCC = "Resent-Bcc"
)

var (
	nonPermittedSenderRe = regexp.MustCompile("(^owner-|-service|^MAILER-DAEMON)@")

	// headers used to detect list membership
	listHeaders = []string{
		"List-Help",
		"List-Unsubscribe",
		"List-Subscribe",
		"List-Post",
		"List-Owner",
		"List-Archive",
	}

	// headers used as recipient addresses
	recipientAddressHeaders = []string{
		gomail.HeaderTo.String(),
		gomail.HeaderCc.String(),
		gomail.HeaderBcc.String(),
		headerResentTo,
		headerResentCC,
		headerResentBCC,
	}
)

// New creates a backend accepting mails for vacation domain over lmtp. When sending replies, smtp server at server:port is contacted.
// Ratelimiter limits rate of sending replies. Replygenerator generates reply body message.
func New(domain string, smtpserver string, smtpport int, ratelimiter interfaces.RateLimiter, replygenerator interfaces.ReplyGenerator) (smtp.Backend, error) {
	return &backend{
		domain:         domain,
		server:         smtpserver,
		port:           smtpport,
		ratelimiter:    ratelimiter,
		replygenerator: replygenerator,
	}, nil
}

type backend struct {
	domain         string
	server         string
	port           int
	ratelimiter    interfaces.RateLimiter
	replygenerator interfaces.ReplyGenerator
}

func (b *backend) handleMail(ctx context.Context, msg *mail.Message, sender string, recipient map[string]string, status smtp.StatusCollector) (err error) {
	// check for auto-submitted message
	if isAutoSubmitted(msg) {
		return nil
	}

	// Check sender
	if !isPermittedSender(sender) {
		return nil
	}

	// Check if response would be appropriate
	if !isResponseAppropriate(msg) {
		return nil
	}

	// process each recipient
	for rcpt, rcptreal := range recipient {
		status.SetStatus(rcpt, b.handleMailSingle(ctx, msg, sender, rcptreal))
	}

	return nil
}

func (b *backend) handleMailSingle(ctx context.Context, msg *mail.Message, sender string, recipient string) (err error) {
	// Check if addressed to mailbox
	if !isMessageAddressedTo(msg, recipient) {
		log.Printf("Not generating reply for message <%s> -> <%s>: not addressed to us", sender, recipient)
		return nil
	}

	// Do rate-limiting
	if !b.ratelimiter.Ratelimit(recipient, sender) {
		log.Printf("Not generating reply for message <%s> -> <%s>: rate-limited", sender, recipient)
		return
	}

	// Generate reply
	reply, err := b.GenerateReply(ctx, sender, recipient, msg)
	if err != nil {
		return
	}

	if reply == nil {
		log.Printf("Not sending reply for message <%s> -> <%s>: empty response from replygenerator", sender, recipient)
		return
	}

	log.Printf("Sending reply for message <%s> -> <%s>", sender, recipient)

	c, err := gomail.NewClient(b.server, gomail.WithPort(b.port), gomail.WithTLSPolicy(gomail.NoTLS))
	if err != nil {
		return
	}
	defer c.Close()

	return c.DialAndSendWithContext(ctx, reply)
}

func (b *backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &session{
		c:        c,
		backend:  b,
		rcptreal: make(map[string]string),
	}, nil
}

// extractAddress extracts looks for address in listed fields, and
// returns full string on match, otherwise just the original address
func extractAddress(msg *mail.Message, address string, fields ...string) string {
	for _, f := range fields {
		if addrs, err := msg.Header.AddressList(f); err == nil {
			if len(addrs) > 0 && addrs[0].Address == address {
				return addrs[0].String()
			}
		}
	}

	return address
}

// GenerateReply takes original envelope from/rcpt and message
func (b *backend) GenerateReply(ctx context.Context,
	originalEnvelopeSender,
	originalEnvelopeRecipient string,
	msg *mail.Message) (m *gomail.Msg, err error) {
	m = gomail.NewMsg(gomail.WithCharset(gomail.CharsetUTF8))

	// Fill-in from/to
	if err = m.From(extractAddress(msg, originalEnvelopeRecipient, recipientAddressHeaders...)); err != nil {
		return
	}

	if err = m.To(extractAddress(msg, originalEnvelopeSender, gomail.HeaderFrom.String())); err != nil {
		return
	}

	// Date
	m.SetDate()

	// Subject
	m.Subject(fmt.Sprintf("Auto: %s", msg.Header.Get(gomail.HeaderSubject.String())))

	// Message-ID
	m.SetMessageID()

	// Set References/In-Reply-To
	var msgid string
	if msgid = msg.Header.Get(gomail.HeaderMessageID.String()); msgid != "" {
		m.SetGenHeader(gomail.HeaderInReplyTo, msgid)
	}
	var references string
	if references = msg.Header.Get(referencesHeader); references == "" {
		references = msg.Header.Get(gomail.HeaderInReplyTo.String())
	}
	var newrefs []string
	if references != "" {
		newrefs = append(newrefs, references)
	}
	if msgid != "" {
		newrefs = append(newrefs, msgid)
	}
	if len(newrefs) > 0 {
		m.SetGenHeader(referencesHeader, strings.Join(newrefs, " "))
	}

	// Mark as generated, try to suppress replies
	m.SetGenHeader(autoSubmittedHeader, autoSubmittedHeaderAutoReplied)
	m.SetGenHeader(xAutoResponseSuppressHeader, xAutoResponseSuppressHeaderAllValue)

	// Generate/alter reply
	if b.replygenerator != nil {
		m, err = b.replygenerator.GenerateReplyBody(ctx, msg, originalEnvelopeSender, originalEnvelopeRecipient, m)
	} else {
		m.SetBodyString(gomail.TypeTextPlain, "Reader of mailbox is out-of-office / on vacation")
	}

	return
}
