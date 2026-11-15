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

// Every way a share body can be wrong is the same refusal, a stream that breaks included.
func TestReadShareBody(t *testing.T) {
	for name, body := range map[string]io.Reader{
		"no body":        nil,
		"too large":      strings.NewReader(`{"packId":"` + strings.Repeat("p", maxShareBodyBytes) + `"}`),
		"broken stream":  brokenBody{},
		"not an object":  strings.NewReader(`[]`),
		"unknown member": strings.NewReader(`{"packId":"p","other":1}`),
		"no pack":        strings.NewReader(`{"listed":true}`),
		"empty pack":     strings.NewReader(`{"packId":""}`),
		"null listed":    strings.NewReader(`{"packId":"p","listed":null}`),
	} {
		if _, _, ok := readShareBody(bodyRequest(body)); ok {
			t.Errorf("%s: read", name)
		}
	}
	packID, listed, ok := readShareBody(bodyRequest(strings.NewReader(`{"packId":"p","listed":false}`)))
	if !ok || packID != "p" || listed == nil || *listed {
		t.Fatalf("packID = %q, listed = %v, ok = %v", packID, listed, ok)
	}
	if _, listed, ok := readShareBody(bodyRequest(strings.NewReader(`{"packId":"p"}`))); !ok || listed != nil {
		t.Fatalf("listed = %v, ok = %v", listed, ok)
	}
}

func TestReadStringFields(t *testing.T) {
	for name, body := range map[string]io.Reader{
		"no body":        nil,
		"too large":      strings.NewReader(`{"id":"` + strings.Repeat("i", maxIDBodyBytes) + `"}`),
		"broken stream":  brokenBody{},
		"not an object":  strings.NewReader(`"id"`),
		"missing member": strings.NewReader(`{}`),
		"extra member":   strings.NewReader(`{"id":"a","other":"b"}`),
		"not a string":   strings.NewReader(`{"id":1}`),
		"null":           strings.NewReader(`{"id":null}`),
		"empty":          strings.NewReader(`{"id":""}`),
	} {
		if _, ok := readStringFields(bodyRequest(body), maxIDBodyBytes, "id"); ok {
			t.Errorf("%s: read", name)
		}
	}
	fields, ok := readStringFields(bodyRequest(strings.NewReader(`{"id":"a","reason":"spam"}`)), maxIDBodyBytes, "id", "reason")
	if !ok || fields["id"] != "a" || fields["reason"] != "spam" {
		t.Fatalf("fields = %v, ok = %v", fields, ok)
	}
}
