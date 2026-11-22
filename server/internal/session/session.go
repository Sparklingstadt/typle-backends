// Package session opens the login cookie and the iPad and Mac apps' bearer token, as src/server/session.ts does. Both
// are base64url(JSON) + "." + base64url(HMAC-SHA256 of that text), without padding, under a key derived from
// AUTH_SECRET with HKDF (a different key for each kind, so one never opens as the other). Only opening is served here:
// Next.js signs the users in (src/server/auth.ts), and Seal is kept for the tests.
package session

import (
	"bytes"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// MaxAge is how long a session is valid after its login (SESSION_MAX_AGE).
	MaxAge = 30 * 24 * time.Hour

	sessionInfo = "typle session v1"
	appInfo     = "typle app session v1"
)

// accountSub is ACCOUNT_SUB of src/server/session.ts: a GitHub account (its numeric id), an ID and password account
// (a lowercase UUID), an FFPF ID account (its lowercase alphanumeric sub) or a Sparklingstadt ID account (sid:<sub>).
var accountSub = regexp.MustCompile(`^(github:\d+|password:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|ffpf:[a-z0-9]{1,64}|sid:[a-z0-9]{1,64})$`)

// Session is a signed-in user. IssuedAt and Expires are the iat and exp claims, in Unix seconds, as the JavaScript
// numbers src/server/session.ts reads them as.
type Session struct {
	Sub, Name, Image  string
	IssuedAt, Expires float64
}

// Keys holds the key of the login cookie and that of the app token. The zero Keys opens nothing.
type Keys struct {
	session, app []byte
}

// NewKeys derives the keys from AUTH_SECRET as deriveKey in src/server/settings.ts does. An empty secret would make
// every cookie forgeable, so it is refused.
func NewKeys(secret string) (Keys, error) {
	if secret == "" {
		return Keys{}, errors.New("AUTH_SECRET is empty")
	}
	sessionKey, err := hkdf.Key(sha256.New, []byte(secret), nil, sessionInfo, sha256.Size)
	if err != nil {
		return Keys{}, err
	}
	appKey, err := hkdf.Key(sha256.New, []byte(secret), nil, appInfo, sha256.Size)
	if err != nil {
		return Keys{}, err
	}
	return Keys{session: sessionKey, app: appKey}, nil
}

// New starts a session at now (whole seconds), as newSession does.
func New(sub, name, image string, now time.Time) Session {
	issued := float64(now.Unix())
	return Session{Sub: sub, Name: name, Image: image, IssuedAt: issued, Expires: issued + MaxAge.Seconds()}
}

// RevokedBy is revokedBy of src/server/account.ts: signing out of every device at revokedAt (read as a JavaScript Date,
// to the millisecond) revoked s when s's login is no later than it; a login in that very second counts too.
func (s Session) RevokedBy(revokedAt time.Time) bool {
	return !revokedAt.IsZero() && s.IssuedAt*1000 <= float64(revokedAt.UnixMilli())
}

// CookieName is sessionCookieName: __Host- over HTTPS, which makes the browser insist on Secure, Path=/ and no Domain.
func CookieName(secure bool) string {
	if secure {
		return "__Host-typle-session"
	}
	return "typle-session"
}

// Seal signs s as the login cookie, as sealSession does. It panics on the zero Keys.
func (k Keys) Seal(s Session) string { return seal(k.session, s) }

// SealApp signs s as an app's token. It panics on the zero Keys.
func (k Keys) SealApp(s Session) string { return seal(k.app, s) }

// Open returns the session in the cookie value if it was sealed with the cookie's key, has not expired at now, and is
// for a GitHub or FFPF ID account (openSession).
func (k Keys) Open(value string, now time.Time) (Session, bool) {
	return openSession(k.session, value, now)
}

// OpenApp is Open for an app's token.
func (k Keys) OpenApp(value string, now time.Time) (Session, bool) {
	return openSession(k.app, value, now)
}

// openSession reads the claims as openSession of src/server/session.ts does: sub, name and image must be strings, iat
// and exp integers (Number.isInteger: 1.0 and 1e9 are), names are matched exactly and the last of a repeated name wins.
func openSession(key []byte, value string, now time.Time) (Session, bool) {
	claims, ok := openClaims(key, value)
	if !ok {
		return Session{}, false
	}
	sub, okSub := stringClaim(claims["sub"])
	name, okName := stringClaim(claims["name"])
	image, okImage := stringClaim(claims["image"])
	iat, okIat := integerClaim(claims["iat"])
	exp, okExp := integerClaim(claims["exp"])
	if !okSub || !okName || !okImage || !okIat || !okExp {
		return Session{}, false
	}
	// Date.now() / 1000, as the TypeScript side reads the clock.
	if !(float64(now.UnixMilli())/1000 < exp) || !accountSub.MatchString(sub) {
		return Session{}, false
	}
	return Session{Sub: sub, Name: name, Image: image, IssuedAt: iat, Expires: exp}, true
}

// openClaims is the JSON object sealed in value with key: the signature and payload must be strict base64url (one
// spelling for one value), the signature the HMAC of the payload as received.
func openClaims(key []byte, value string) (map[string]json.RawMessage, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || len(key) == 0 {
		return nil, false
	}
	payload, signature := parts[0], parts[1]
	got, err := base64.RawURLEncoding.Strict().DecodeString(signature)
	if err != nil || !hmac.Equal(got, sign(key, payload)) {
		return nil, false
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(payload)
	if err != nil {
		return nil, false
	}
	var claims map[string]json.RawMessage
	if json.Unmarshal(data, &claims) != nil || claims == nil {
		return nil, false
	}
	return claims, true
}

func stringClaim(raw json.RawMessage) (string, bool) {
	var value string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

func integerClaim(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 || (raw[0] != '-' && (raw[0] < '0' || raw[0] > '9')) {
		return 0, false
	}
	number, err := strconv.ParseFloat(string(raw), 64)
	if err != nil || math.IsInf(number, 0) || number != math.Trunc(number) {
		return 0, false
	}
	return number, true
}

// seal writes the claims as encodeClaims of src/server/session.ts: JSON in the order sub, name, image, iat, exp, with
// & < > as they are.
func seal(key []byte, s Session) string {
	if len(key) == 0 {
		panic("session: sealing with an empty key")
	}
	wire := struct {
		Sub   string `json:"sub"`
		Name  string `json:"name"`
		Image string `json:"image"`
		Iat   int64  `json:"iat"`
		Exp   int64  `json:"exp"`
	}{s.Sub, s.Name, s.Image, int64(s.IssuedAt), int64(s.Expires)}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(wire)
	payload := base64.RawURLEncoding.EncodeToString(bytes.TrimSuffix(buffer.Bytes(), []byte("\n")))
	return payload + "." + base64.RawURLEncoding.EncodeToString(sign(key, payload))
}

func sign(key []byte, payload string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}
