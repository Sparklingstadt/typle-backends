package session

import "testing"

// Values better-call's signCookieValue (better-call 1.x in typle-r v2.68.1) signed with this secret.
const fixtureSecret = "typle-fixture-secret"

var signed = []struct{ token, value string }{
	{"Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj", "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D"},
	{"a.b+c/d=e", "a.b%2Bc%2Fd%3De.QgtT95hjCvQfprQC%2BUtI5LsqCC549MK9KWYHFNPmUuE%3D"},
}

func TestSignAsBetterCall(t *testing.T) {
	for _, c := range signed {
		if got := Sign(c.token, fixtureSecret); got != c.value {
			t.Errorf("Sign(%q) = %q, want %q", c.token, got, c.value)
		}
	}
}

func TestToken(t *testing.T) {
	for _, c := range signed {
		if token, ok := Token(c.value, fixtureSecret); !ok || token != c.token {
			t.Errorf("Token(%q) = %q, %v", c.value, token, ok)
		}
	}
	// The value as it is, not URL-encoded, opens too: decoding leaves it as it is.
	if token, ok := Token("Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK+RyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w=", fixtureSecret); !ok || token != signed[0].token {
		t.Errorf("decoded value = %q, %v", token, ok)
	}
	// A value that is not valid percent-encoding is read as it is (tryDecode).
	if _, ok := Token("%zz."+signed[0].value, fixtureSecret); ok {
		t.Error("a value with bad escapes and a wrong signature opened")
	}
}

func TestTokenRefused(t *testing.T) {
	good := signed[0].value
	for name, value := range map[string]string{
		"empty":          "",
		"no dot":         "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj",
		"dot first":      ".HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D",
		"other token":    "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHk.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D",
		"short":          "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w",
		"no padding end": "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2wA",
		"not base64":     "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK-RyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D",
	} {
		if token, ok := Token(value, fixtureSecret); ok {
			t.Errorf("%s: opened as %q", name, token)
		}
	}
	if _, ok := Token(good, "another secret"); ok {
		t.Error("opened under another secret")
	}
	if _, ok := Token(good, ""); ok {
		t.Error("opened under an empty secret")
	}
}

func TestCookieName(t *testing.T) {
	if CookieName(false) != "better-auth.session_token" || CookieName(true) != "__Secure-better-auth.session_token" {
		t.Fatal(CookieName(false), CookieName(true))
	}
}
