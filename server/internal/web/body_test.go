package web

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// bodyRequest is a POST whose body is body; nil sends none at all.
func bodyRequest(body io.Reader) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", body)
	if body == nil {
		r.Body = http.NoBody
	}
	return r
}

func TestReadObject(t *testing.T) {
	for name, test := range map[string]struct {
		body io.Reader
		want error
	}{
		"no body":       {nil, errNoBody},
		"too large":     {strings.NewReader(`{"a":"` + strings.Repeat("x", 20) + `"}`), errTooLarge},
		"not json":      {strings.NewReader(`{`), errNotObject},
		"null":          {strings.NewReader(`null`), errNotObject},
		"array":         {strings.NewReader(`[]`), errNotObject},
		"broken stream": {brokenBody{}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := readObject(bodyRequest(test.body), 16)
			if err == nil {
				t.Fatal("read")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("err = %v, want %v", err, test.want)
			}
			if test.want == nil && (errors.Is(err, errNoBody) || errors.Is(err, errTooLarge) || errors.Is(err, errNotObject)) {
				t.Fatalf("a broken stream is %v", err)
			}
		})
	}
	fields, err := readObject(bodyRequest(strings.NewReader(`{"a":1}`)), 16)
	if err != nil || string(fields["a"]) != "1" {
		t.Fatalf("fields = %v, err = %v", fields, err)
	}
}
