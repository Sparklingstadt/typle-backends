package web

import (
	"net/url"

	"github.com/sparklingstadt/typle-r/server/internal/session"
)

// LoginFromEnv reads the login settings as readLoginSettings of src/server/settings.ts does: AUTH_SECRET must not be
// empty and AUTH_URL must be an http or https URL with a host, of which only the origin is used. ok is false otherwise,
// which the routes answer with 503 authUnavailable.
func LoginFromEnv(getenv func(string) string) (keys session.Keys, base *url.URL, ok bool) {
	secret := getenv("AUTH_SECRET")
	parsed, err := url.Parse(getenv("AUTH_URL"))
	if secret == "" || err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return session.Keys{}, nil, false
	}
	keys, err = session.NewKeys(secret)
	if err != nil {
		return session.Keys{}, nil, false
	}
	return keys, &url.URL{Scheme: parsed.Scheme, Host: parsed.Host}, true
}
