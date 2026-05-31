package auth

import (
	"context"
	"crypto/subtle"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/vlourme/go-proxy/internal/config"
)

// Authenticator verifies proxy credentials.
type Authenticator interface {
	Verify(username, password string) bool
}

// NewAuthenticator returns the appropriate authenticator for the config.
func NewAuthenticator(cfg *config.Config) (Authenticator, error) {
	switch cfg.Auth.Type {
	case config.AuthTypeNone:
		return &NoneAuthenticator{}, nil
	case config.AuthTypeCredentials:
		return &CredentialsAuthenticator{
			Username: cfg.Auth.Credentials.Username,
			Password: cfg.Auth.Credentials.Password,
		}, nil
	case config.AuthTypeRedis:
		client, err := NewRedisClient(cfg.Auth.Redis.DSN)
		if err != nil {
			return nil, fmt.Errorf("redis auth: %w", err)
		}
		return &RedisAuthenticator{client: client}, nil
	default:
		return &NoneAuthenticator{}, nil
	}
}

// NoneAuthenticator allows all requests.
type NoneAuthenticator struct{}

func (a *NoneAuthenticator) Verify(_, _ string) bool { return true }

// CredentialsAuthenticator checks against static credentials.
type CredentialsAuthenticator struct {
	Username string
	Password string
}

func (a *CredentialsAuthenticator) Verify(username, password string) bool {
	if username == "" || password == "" {
		return false
	}
	if a.Username == "" || a.Password == "" {
		return false
	}
	userMatch := subtle.ConstantTimeCompare([]byte(username), []byte(a.Username)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(password), []byte(a.Password)) == 1
	return userMatch && passMatch
}

// RedisAuthenticator checks credentials against Redis.
type RedisAuthenticator struct {
	client *redis.Client
}

func (a *RedisAuthenticator) Verify(username, password string) bool {
	if username == "" || password == "" {
		return false
	}
	val, err := a.client.Get(context.Background(), username).Result()
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(val), []byte(password)) == 1
}
