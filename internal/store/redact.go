package store

import (
	"errors"
	"regexp"
)

// urlPassword matches the userinfo password of a connection URL.
var urlPassword = regexp.MustCompile(`(://[^:/@]+:)[^@]*@`)

// redact replaces any password inside a connection URL that pgx may have
// echoed into an error message. pgx errors normally name only the host, but
// the parse path can quote the input.
func redact(err error) error {
	msg := urlPassword.ReplaceAllString(err.Error(), "${1}***@")
	if msg == err.Error() {
		return err
	}
	return errors.New(msg)
}
