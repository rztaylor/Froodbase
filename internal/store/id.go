package store

import (
	"crypto/rand"
	"fmt"
)

// NewID returns a fresh record identifier: a random (version 4) UUID in its
// canonical text form, which is what every id column holds. Callers build
// domain records with it before handing them to the writer.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("store: reading random bytes: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
