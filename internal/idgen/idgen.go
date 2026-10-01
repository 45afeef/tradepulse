// Package idgen produces random, URL-safe identifiers.
package idgen

import (
	"crypto/rand"
	"encoding/hex"
)

// New returns a 128-bit random hex identifier.
func New() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("idgen: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}
