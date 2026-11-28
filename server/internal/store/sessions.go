package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sparklingstadt/typle-backends/server/internal/session"
)

// The sessions are Better Auth's rows (typle_auth_sessions, migration 0042 of typle-r): the login cookie names one by
// its token, and signing out of every device deletes the user's rows.

// SessionUser is the user of the session whose token is token, as Better Auth's findSession and getSession read it:
// the session's row and its user's, and only while the session has not expired at now (expires_at not before now).
// ok is false for a token of no session and for an expired one.
func (s *Store) SessionUser(ctx context.Context, token string, now time.Time) (user session.Session, ok bool, err error) {
	var image *string
	err = s.pool.QueryRow(ctx, `SELECT u.id, u.name, u.image FROM typle_auth_sessions s
		JOIN typle_auth_users u ON u.id = s.user_id WHERE s.token = $1 AND s.expires_at >= $2`, token, now).
		Scan(&user.Sub, &user.Name, &image)
	if errors.Is(err, pgx.ErrNoRows) {
		return session.Session{}, false, nil
	}
	if err != nil {
		return session.Session{}, false, err
	}
	if image != nil {
		user.Image = *image
	}
	return user, true, nil
}
