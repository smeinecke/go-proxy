package auth

import (
	"context"
	"crypto/subtle"

	"github.com/vlourme/go-proxy/internal/config"
)

// Verify verifies the credentials of the user.
// AuthTypeNone allows any request through without credentials.
func Verify(username, password string) bool {
	switch config.Get().Auth.Type {
	case config.AuthTypeNone:
		return true
	case config.AuthTypeCredentials, config.AuthTypeRedis:
		if username == "" || password == "" {
			return false
		}
	}

	switch config.Get().Auth.Type {
	case config.AuthTypeCredentials:
		return verifyCredentials(username, password)
	case config.AuthTypeRedis:
		return verifyRedisCredentials(username, password)
	default:
		return false
	}
}

// verifyCredentials verifies the credentials of the user using constant-time comparison.
func verifyCredentials(username, password string) bool {
	cfgUsername, cfgPassword := config.Get().Auth.Credentials.Username, config.Get().Auth.Credentials.Password

	if cfgUsername == "" || cfgPassword == "" {
		return false
	}

	userMatch := subtle.ConstantTimeCompare([]byte(username), []byte(cfgUsername)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(password), []byte(cfgPassword)) == 1
	return userMatch && passMatch
}

// verifyRedisCredentials verifies the credentials of the user using Redis
func verifyRedisCredentials(username, password string) bool {
	client := GetRedisClient()

	val, err := client.Get(context.Background(), username).Result()
	if err != nil {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(val), []byte(password)) == 1
}
