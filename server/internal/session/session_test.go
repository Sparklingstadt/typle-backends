package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fixture reads a file of tests/fixtures, which tests/serverSession.test.cjs reads too.
func fixture(t *testing.T, name string, v any) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "tests", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}

type claims struct {
	Sub   string  `json:"sub"`
	Name  string  `json:"name"`
	Image string  `json:"image"`
	Iat   float64 `json:"iat"`
	Exp   float64 `json:"exp"`
}

func claimsOf(s Session, ok bool) *claims {
	if !ok {
		return nil
	}
	return &claims{s.Sub, s.Name, s.Image, s.IssuedAt, s.Expires}
}

type sessionsFixture struct {
	Secret  string `json:"secret"`
	Now     int64  `json:"now"`
	Cookies []struct {
		Value   string  `json:"value"`
		Session *claims `json:"session"`
	} `json:"cookies"`
	Tokens []struct {
		Value   string  `json:"value"`
		Session *claims `json:"session"`
	} `json:"tokens"`
}

func equal(a, b *claims) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

// The cookies and tokens of tests/fixtures/sessions.json open as src/server/session.ts opens them.
func TestSessionsFixture(t *testing.T) {
	var f sessionsFixture
	fixture(t, "sessions.json", &f)
	keys, err := NewKeys(f.Secret)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(f.Now, 0)
	for _, c := range f.Cookies {
		if got := claimsOf(keys.Open(c.Value, now)); !equal(got, c.Session) {
			t.Errorf("cookie %s = %+v; want %+v", c.Value, got, c.Session)
		}
		if _, ok := keys.OpenApp(c.Value, now); ok {
			t.Errorf("cookie %s opens as a token", c.Value)
		}
	}
	for _, c := range f.Tokens {
		if got := claimsOf(keys.OpenApp(c.Value, now)); !equal(got, c.Session) {
			t.Errorf("token %s = %+v; want %+v", c.Value, got, c.Session)
		}
		if _, ok := keys.Open(c.Value, now); ok {
			t.Errorf("token %s opens as a cookie", c.Value)
		}
	}
	other, _ := NewKeys("another-secret")
	if _, ok := other.Open(f.Cookies[0].Value, now); ok {
		t.Error("a cookie opens under another secret")
	}
	// Expiry is exclusive.
	first := f.Cookies[0]
	if _, ok := keys.Open(first.Value, time.Unix(int64(first.Session.Exp)-1, 0)); !ok {
		t.Error("closed before it expires")
	}
	if _, ok := keys.Open(first.Value, time.Unix(int64(first.Session.Exp), 0)); ok {
		t.Error("opens when it expires")
	}
}

// The values of tests/fixtures/auth-answers.json are sealed with the same bytes as sealSession seals them.
func TestAuthAnswersSeal(t *testing.T) {
	var f struct {
		Secret string            `json:"secret"`
		Now    int64             `json:"now"`
		Values map[string]string `json:"values"`
	}
	fixture(t, "auth-answers.json", &f)
	keys, _ := NewKeys(f.Secret)
	login := float64(f.Now - 100)
	month := MaxAge.Seconds()
	alice := Session{"github:9100000001", "Alice", "https://avatars.example/u/1", login, login + month}
	if got := keys.Seal(alice); got != f.Values["alice"] {
		t.Errorf("alice = %s", got)
	}
	if got := keys.SealApp(alice); got != f.Values["alice-app"] {
		t.Errorf("alice-app = %s", got)
	}
	bob := Session{"github:9100000002", "<b&o> b  😀", "", login, login + month}
	opened, ok := keys.Open(f.Values["bob"], time.Unix(f.Now, 0))
	if !ok || opened != bob {
		t.Errorf("bob = %+v, %v", opened, ok)
	}
	// encodeClaims writes U+2028 and U+2029 escaped, as Go's encoder does.
	if got := keys.Seal(bob); got != f.Values["bob"] {
		t.Errorf("bob sealed = %s", got)
	}
}

// sealRaw signs payload JSON as it is, for claims of the wrong types.
func sealRaw(key []byte, payload string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// The claims are read as openSession reads them with JSON.parse, not as encoding/json would: names exactly, strings
// and integers only, and an object.
func TestOpenClaimsAsTypeScript(t *testing.T) {
	keys, _ := NewKeys("secret")
	now := time.Unix(2000, 0)
	for payload, want := range map[string]bool{
		`{"sub":"github:1","name":"N","image":"","iat":1000,"exp":3000}`:                  true,
		`{"sub":"github:1","name":"N","image":"","iat":1000.0,"exp":3e3}`:                 true,
		`{"sub":"github:1","name":"N","image":"","iat":1000,"exp":3000,"x":1}`:            true,
		`{"sub":"github:2","sub":"github:1","name":"N","image":"","iat":1000,"exp":3000}`: true,
		`{"sub":"github:1","name":"N","iat":1000,"exp":3000}`:                             false, // no image
		`{"sub":"github:1","name":null,"image":"","iat":1000,"exp":3000}`:                 false,
		`{"SUB":"github:1","name":"N","image":"","iat":1000,"exp":3000}`:                  false,
		`{"sub":"github:1","name":"N","image":"","iat":1000.5,"exp":3000}`:                false,
		`{"sub":"github:1","name":"N","image":"","iat":"1000","exp":3000}`:                false,
		`{"sub":"github:1","name":"N","image":"","iat":1000,"exp":2000}`:                  false, // expired at now
		`{"sub":"github:1","name":"N","image":"","iat":1000,"exp":1e400}`:                 false,
		`{"sub":"x:1","name":"N","image":"","iat":1000,"exp":3000}`:                       false,
		`{"sub":"github:1\n","name":"N","image":"","iat":1000,"exp":3000}`:                false,
		`null`: false,
		`[]`:   false,
	} {
		_, ok := keys.Open(sealRaw(keys.session, payload), now)
		if ok != want {
			t.Errorf("%s: opened %v; want %v", payload, ok, want)
		}
	}
}

func TestOpenRefusesMalformed(t *testing.T) {
	keys, _ := NewKeys("secret")
	now := time.Unix(1000, 0)
	value := keys.Seal(New("github:1", "N", "", now))
	payload, signature, _ := strings.Cut(value, ".")
	flip := func(text string) string {
		last := "A"
		if strings.HasSuffix(text, "A") {
			last = "B"
		}
		return text[:len(text)-1] + last
	}
	for _, changed := range []string{payload + "." + flip(signature), flip(payload) + "." + signature, value + "=",
		payload + "=." + signature, payload, "", ".", value + ".x", value[:len(value)-1]} {
		if _, ok := keys.Open(changed, now); ok {
			t.Errorf("%q opened", changed)
		}
	}
	if _, ok := (Keys{}).Open(value, now); ok {
		t.Error("the zero Keys opened a cookie")
	}
	if _, err := NewKeys(""); err == nil {
		t.Error("an empty secret was taken")
	}
}

// revokedBy: a login no later than the revocation, to the millisecond of a JavaScript Date, is revoked.
func TestRevokedBy(t *testing.T) {
	s := New("github:1", "", "", time.Unix(1000, 0))
	for _, c := range []struct {
		at   time.Time
		want bool
	}{
		{time.Time{}, false},
		{time.Unix(999, 999_999_999), false},
		{time.Unix(1000, 0), true},
		{time.Unix(1000, 999), true},
		{time.Unix(1001, 0), true},
	} {
		if got := s.RevokedBy(c.at); got != c.want {
			t.Errorf("RevokedBy(%v) = %v", c.at, got)
		}
	}
}

func TestCookieName(t *testing.T) {
	if CookieName(true) != "__Host-typle-session" || CookieName(false) != "typle-session" {
		t.Fatal(CookieName(true), CookieName(false))
	}
}

// A Sparklingstadt ID account (sid:<sub>, the sub lowercase alphanumeric) opens as in TypeScript.
func TestOpenSidSubject(t *testing.T) {
	keys, _ := NewKeys("secret")
	now := time.Unix(1000, 0)
	for sub, want := range map[string]bool{
		"sid:cm3abc123":                  true,
		"sid:" + strings.Repeat("a", 64): true,
		"sid:":                           false,
		"sid:ABC":                        false,
		"sid:a-b":                        false,
		"sid:" + strings.Repeat("a", 65): false,
		"SID:abc":                        false,
	} {
		if _, ok := keys.Open(keys.Seal(New(sub, "N", "", now)), now); ok != want {
			t.Errorf("%q: opened %v; want %v", sub, ok, want)
		}
	}
}
