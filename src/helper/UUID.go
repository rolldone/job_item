package helper

import (
	"github.com/gofrs/uuid"
)

// GenerateUUIDv7 generates a UUID version 7 (time-ordered).
func GenerateUUIDv7() (string, error) {
	u, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
