package secret

import (
	"crypto/rand"
	"encoding/base64"
)

func RandomToken() string {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(value[:])
}
