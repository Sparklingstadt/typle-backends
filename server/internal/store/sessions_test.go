package store

import (
	"context"
	"testing"
	"time"
)

// A session row of a user, as Better Auth writes them, is read by its token while it has not expired.
func TestSessionUser(t *testing.T) {
	ctx := context.Background()
	connection, conn := database(t)
	s := open(t, connection)
	id := owner(t, conn)
	token := "token-" + id
	t.Cleanup(func() {
		// The session row goes with its user (ON DELETE CASCADE).
		_, _ = conn.Exec(context.Background(), `DELETE FROM typle_auth_users WHERE id = $1`, id)
	})
	now := time.Now()
	if _, err := conn.Exec(ctx, `INSERT INTO typle_auth_users (id, name, email, email_verified) VALUES ($1, 'Alice', $2, true)`,
		id, id+"@users.invalid"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO typle_auth_sessions (id, token, user_id, expires_at) VALUES ($1, $2, $3, $4)`,
		"session-"+id, token, id, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	user, ok, err := s.SessionUser(ctx, token, now)
	if err != nil || !ok || user.Sub != id || user.Name != "Alice" || user.Image != "" {
		t.Fatalf("SessionUser = %+v, %v, %v", user, ok, err)
	}
	if _, ok, err := s.SessionUser(ctx, token+"x", now); err != nil || ok {
		t.Fatalf("unknown token = %v, %v", ok, err)
	}
	if _, ok, err := s.SessionUser(ctx, token, now.Add(2*time.Hour)); err != nil || ok {
		t.Fatalf("expired = %v, %v", ok, err)
	}
	// Signing out of every device deletes the rows: the cookie no longer opens.
	if _, err := conn.Exec(ctx, `DELETE FROM typle_auth_sessions WHERE user_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.SessionUser(ctx, token, now); err != nil || ok {
		t.Fatalf("after the sessions were deleted = %v, %v", ok, err)
	}
}
