package store

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// ConnString keeps the server certificate verified, as the app's database connection did before the Go
// backend: sslmode=prefer, require and verify-ca become verify-full. uselibpqcompat=true, which asks node-postgres
// for libpq's meaning of those modes, is pgx's own behaviour, so it keeps the mode and only the parameter is dropped.
// A URL without sslmode (or with an empty one) is verify-full too, unless every host it names is this machine (localHosts): pgx's default,
// prefer, neither checks the certificate nor insists on TLS at all.
func ConnString(connection string) (string, error) {
	parsed, err := url.Parse(connection)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return "", errors.New("database URL must be a postgres:// or postgresql:// URL")
	}
	query := parsed.Query()
	libpq := query.Get("uselibpqcompat") == "true"
	query.Del("uselibpqcompat")
	if !libpq {
		switch query.Get("sslmode") {
		case "prefer", "require", "verify-ca":
			query.Set("sslmode", "verify-full")
		case "":
			if !localHosts(parsed, query) {
				query.Set("sslmode", "verify-full")
			}
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// localHosts reports whether every host the URL connects to is this machine: localhost, 127.0.0.0/8, ::1, a Unix
// socket directory, or no host at all (pgx's default socket). The host query parameter, as pgx reads it, wins over
// the URL's, and either may list several hosts separated by commas.
func localHosts(parsed *url.URL, query url.Values) bool {
	hosts := query.Get("host")
	if hosts == "" {
		hosts = parsed.Host
	}
	for _, host := range strings.Split(hosts, ",") {
		if strings.HasPrefix(host, "/") {
			continue
		}
		if name, _, err := net.SplitHostPort(host); err == nil {
			host = name
		}
		host = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"))
		if host == "" || host == "localhost" {
			continue
		}
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			return false
		}
	}
	return true
}
