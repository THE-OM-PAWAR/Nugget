package config

import (
	"crypto/rand"
	"fmt"
	"io"
)

// GenerateMachineID generates a cryptographically random RFC 4122 compliant UUID v4.
func GenerateMachineID() (string, error) {
	var uuid [16]byte
	if _, err := io.ReadFull(rand.Reader, uuid[:]); err != nil {
		return "", fmt.Errorf("failed to read secure random bytes: %w", err)
	}

	// Set version to 4 (0100)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	// Set variant to RFC 4122 (10xx)
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4],
		uuid[4:6],
		uuid[6:8],
		uuid[8:10],
		uuid[10:16],
	), nil
}

