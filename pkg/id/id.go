// Package id generates prefixed, time-sortable identifiers of the form
// "car_01JBQ8Z9K3M4N5P6Q7R8S9T0V1".
package id

import (
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/oklog/ulid/v2"
)

// New returns a prefixed ULID, e.g. New("car") -> "car_01JBQ8...".
func New(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, ulid.MustNew(ulid.Now(), rand.Reader).String())
}

// Prefix returns the entity prefix of an id, or "" if it has none.
func Prefix(value string) string {
	idx := strings.Index(value, "_")
	if idx <= 0 {
		return ""
	}
	return value[:idx]
}

// HasPrefix reports whether value carries the given entity prefix.
func HasPrefix(value, prefix string) bool {
	return Prefix(value) == prefix
}
