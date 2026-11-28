// Package session reads the login cookie that Better Auth sets in typle-r (src/server/betterAuth.ts): the session
// token, signed with BETTER_AUTH_SECRET as better-call's signCookieValue signs it. The cookie only names a session; who
// is signed in, and until when, is the session's row in typle_auth_sessions (store.Store.SessionUser). Only reading is
// served here: typle-r signs the users in, and Sign is kept for the tests.
package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
)

// Session is a signed-in user: Sub is the account ID (typle_auth_users.id, the owner_id of the account's data).
type Session struct {
	Sub, Name, Image string
}

// CookieName is the name of Better Auth's session cookie: better-auth.session_token, with the __Secure- prefix when
// AUTH_URL is HTTPS (createCookieGetter in better-auth's cookies).
func CookieName(secure bool) string {
	if secure {
		return "__Secure-better-auth.session_token"
	}
	return "better-auth.session_token"
}

// signatureLength is the length of an HMAC-SHA256 in standard base64 with its padding, the only length
// better-call's getSignedCookie accepts.
const signatureLength = 44

// Token is the session token of a session cookie's value as the Cookie header carries it, as better-call's
// getSignedCookie reads it: the value is URL-decoded (kept as it is when it cannot be), then split at its last "." into
// the token and its signature, which must be the HMAC-SHA256 of the token under secret in standard base64. ok is false
// for any other value, and for an empty secret.
func Token(value, secret string) (token string, ok bool) {
	if secret == "" {
		return "", false
	}
	if decoded, err := url.PathUnescape(value); err == nil {
		value = decoded
	}
	at := strings.LastIndex(value, ".")
	if at < 1 {
		return "", false
	}
	token, signature := value[:at], value[at+1:]
	if len(signature) != signatureLength || !strings.HasSuffix(signature, "=") {
		return "", false
	}
	sent, err := base64.StdEncoding.DecodeString(signature)
	if err != nil || !hmac.Equal(sent, mac(token, secret)) {
		return "", false
	}
	return token, true
}

// Sign is the cookie value Better Auth sets for token (signCookieValue): the token, ".", its signature, URL-encoded as
// encodeURIComponent encodes it.
func Sign(token, secret string) string {
	return encodeURIComponent(token + "." + base64.StdEncoding.EncodeToString(mac(token, secret)))
}

func mac(token, secret string) []byte {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(token))
	return h.Sum(nil)
}

// encodeURIComponent escapes what JavaScript's encodeURIComponent escapes: every byte but A-Z a-z 0-9 - _ . ! ~ * ' ( ).
func encodeURIComponent(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || strings.IndexByte("-_.!~*'()", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}
