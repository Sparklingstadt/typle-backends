package web

import (
	"net/url"
	"strings"
)

// Login is the login settings of the environment.
type Login struct {
	// Secret is BETTER_AUTH_SECRET, with which Better Auth signs the session cookie; empty when it is not set.
	Secret string
	// BaseURL is the origin of AUTH_URL; nil when AUTH_URL is not an http or https URL with a host.
	BaseURL *url.URL
	// DemoLogin is whether the shared demo account's login is on (DEMO_LOGIN is exactly 1).
	DemoLogin bool
}

// LoginFromEnv reads the login settings as typle-r reads them: AUTH_URL as readLoginSettings of src/server/settings.ts
// (an http or https URL with a host, of which only the origin is used; otherwise the routes answer 503 authUnavailable),
// BETTER_AUTH_SECRET as betterAuthSettings of src/server/authUser.ts (trimmed), and DEMO_LOGIN as demoConfigured.
func LoginFromEnv(getenv func(string) string) Login {
	login := Login{Secret: strings.TrimSpace(getenv("BETTER_AUTH_SECRET")), DemoLogin: getenv("DEMO_LOGIN") == "1"}
	parsed, err := url.Parse(getenv("AUTH_URL"))
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" {
		login.BaseURL = &url.URL{Scheme: parsed.Scheme, Host: parsed.Host}
	}
	return login
}
