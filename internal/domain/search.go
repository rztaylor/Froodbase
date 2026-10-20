package domain

import "strings"

// NameSimilarityThreshold is the trigram similarity at or above which a
// name is a near-miss candidate for the identifier channel (TR-51). A
// candidate is shown to the caller to confirm; it is never resolved to the
// entity on their behalf (D13).
const NameSimilarityThreshold = 0.3

// NormaliseQuery prepares a search query for matching: trimmed, inner
// whitespace collapsed to single spaces. Case is left to the channels,
// which compare case-insensitively.
func NormaliseQuery(q string) string {
	return strings.Join(strings.Fields(q), " ")
}
