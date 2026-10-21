package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// secretPattern is one shape of credential that must never be stored. When
// value is set, it is the submatch holding the credential, and a scrubbed
// placeholder there (***, <redacted>, ${VAR}) is not a secret.
type secretPattern struct {
	what  string
	re    *regexp.Regexp
	value int
}

// secretPatterns are checked in order; the first match names the refusal.
// They aim at what a command's output or a connection string typically
// leaks, not at every possible credential.
var secretPatterns = []secretPattern{
	{"a private key", regexp.MustCompile(`-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----`), 0},
	{"a password in a URL", regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^/\s:@]+:([^@\s/]*)@`), 1},
	{"a password in a connection string", regexp.MustCompile(`(?i)(?:^|[;\s"'{,&?])(?:password|passwd|pwd)["']?\s*[=:]\s*("[^"]*"|'[^']*'|[^;\s"',}&]+)`), 1},
	{"a bearer token", regexp.MustCompile(`(?i)\bbearer\s+([A-Za-z0-9\-._~+/]{16,}=*)`), 1},
	{"basic authentication credentials", regexp.MustCompile(`(?i)\bauthorization["']?\s*[=:]\s*["']?basic\s+([A-Za-z0-9+/]{8,}={0,2})`), 1},
	{"a signed token (JWT)", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{5,}\.eyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}`), 0},
	{"an access token", regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{22,}|xox[abprs]-[A-Za-z0-9-]{10,})`), 0},
	{"a cloud access key", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`), 0},
	{"a key or secret value", regexp.MustCompile(`(?i)\b(?:account[_-]?key|shared[_-]?access[_-]?key|client[_-]?secret|api[_-]?key|access[_-]?key|secret[_-]?key|private[_-]?key|access[_-]?token|refresh[_-]?token|auth[_-]?token|secret|token|sig)["']?\s*[=:]\s*["']?([A-Za-z0-9+/_\-.~%]{16,}={0,2})`), 1},
	{"a storage account key", regexp.MustCompile(`(?:^|[^A-Za-z0-9+/])[A-Za-z0-9+/]{86}==`), 0},
}

// checkSecret reports ErrLooksLikeSecret, saying what kind of secret s
// appears to hold, when it matches a credential pattern. The message never
// repeats the value.
func checkSecret(s string) error {
	if s == "" {
		return nil
	}
	for _, p := range secretPatterns {
		for _, m := range p.re.FindAllStringSubmatch(s, -1) {
			if p.value > 0 && scrubbed(m[p.value]) {
				continue
			}
			return fmt.Errorf("%w (%s); remove it or replace the value with *** before contributing", ErrLooksLikeSecret, p.what)
		}
	}
	return nil
}

// scrubbed reports whether a credential slot holds a placeholder rather
// than a value: nothing, masking characters, a bracketed or variable
// placeholder, or a word such as redacted.
func scrubbed(v string) bool {
	v = strings.Trim(strings.TrimSpace(v), `"'`)
	if v == "" || strings.Trim(v, "*xX•.#-_") == "" {
		return true
	}
	for _, pair := range [][2]string{{"<", ">"}, {"[", "]"}, {"{", "}"}, {"%", "%"}} {
		if strings.HasPrefix(v, pair[0]) && strings.HasSuffix(v, pair[1]) {
			return true
		}
	}
	if strings.HasPrefix(v, "$") || strings.HasPrefix(v, "<") || strings.HasPrefix(v, "{") {
		return true
	}
	switch strings.ToLower(v) {
	case "redacted", "removed", "hidden", "masked", "secret", "none", "null", "password":
		return true
	}
	return false
}
