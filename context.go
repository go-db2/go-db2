package db2

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
)

type contextKey string

const (
	userContextKey contextKey = "db2_session_user"
)

// WithUser returns a copy of parent context with the target session user identity attached.
func WithUser(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, userContextKey, username)
}

// UserFromContext extracts the target session user identity from ctx, or returns empty string if not set.
func UserFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if val, ok := ctx.Value(userContextKey).(string); ok {
		return val
	}
	return ""
}

// SwitchUser transitions the active user identity on a *Conn, *sql.Conn, or driver.Conn.
func SwitchUser(ctx context.Context, target any, newUser string, password ...string) error {
	if target == nil {
		return errors.New("db2: target connection cannot be nil")
	}

	switch c := target.(type) {
	case *Conn:
		return c.SwitchUser(ctx, newUser, password...)
	case driver.Conn:
		if switcher, ok := c.(interface {
			SwitchUser(ctx context.Context, newUser string, password ...string) error
		}); ok {
			return switcher.SwitchUser(ctx, newUser, password...)
		}
	case *sql.Conn:
		return c.Raw(func(driverConn any) error {
			if switcher, ok := driverConn.(interface {
				SwitchUser(ctx context.Context, newUser string, password ...string) error
			}); ok {
				return switcher.SwitchUser(ctx, newUser, password...)
			}
			return errors.New("db2: underlying driver connection does not support SwitchUser")
		})
	case *sql.DB:
		return errors.New("db2: SwitchUser must be called on a dedicated *sql.Conn or *db2.Conn, not on the entire pool *sql.DB")
	}

	return errors.New("db2: unsupported connection type for SwitchUser")
}
