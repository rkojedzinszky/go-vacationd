package backend

import (
	"net/mail"
)

func isAutoSubmitted(msg *mail.Message) bool {
	// https://datatracker.ietf.org/doc/html/rfc3834#section-2
	autosubmitted := msg.Header.Get(autoSubmittedHeader)
	if autosubmitted != "" && autosubmitted != autoSubmittedHeaderNo {
		return true
	}

	// https://learn.microsoft.com/en-us/openspecs/exchange_server_protocols/ms-oxcmail/ced68690-498a-4567-9d14-5c01f974d8b1
	xAutoResponseSuppress := msg.Header.Get(xAutoResponseSuppressHeader)
	if xAutoResponseSuppress == xAutoResponseSuppressHeaderAllValue || xAutoResponseSuppress == xAutoResponseSuppressHeaderOOFValue {
		return true
	}

	return false
}

// isMessageAddressedTo checks if rcpt address is listed in To* or Cc fields.
func isMessageAddressedTo(msg *mail.Message, rcpt string) bool {
	var addrs []*mail.Address
	for _, field := range recipientAddressHeaders {
		if list, err := msg.Header.AddressList(field); err == nil {
			addrs = append(addrs, list...)
		}
	}

	for _, addr := range addrs {
		if addr.Address == rcpt {
			return true
		}
	}

	return false
}

// isPermittedSender checks if sender is a permitted sender
func isPermittedSender(from string) bool {
	return from != "" && !nonPermittedSenderRe.MatchString(from)
}

// isResponseAppropriate checks if reply would be sent to appropriate
// address. Right now it only checks for List-* header presence.
func isResponseAppropriate(msg *mail.Message) bool {
	// check if mail seems to be originated from list
	for _, header := range listHeaders {
		if msg.Header.Get(header) != "" {
			return false
		}
	}

	return true
}
