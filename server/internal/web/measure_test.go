package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sparklingstadt/typle-r/server/internal/store"
)

// meterStore is a Store that keeps the saves it is told of.
type meterStore struct {
	fakeStore
	saves []store.SaveMetric
	fail  error
}

func (m *meterStore) RecordSave(_ context.Context, metric store.SaveMetric) error {
	m.saves = append(m.saves, metric)
	return m.fail
}

// send makes a request with a body; none of these are signed in, so every save is refused before the store is asked.
func send(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(method, path, strings.NewReader(body)))
	return response
}

func TestMeasuredSaves(t *testing.T) {
	meter := &meterStore{}
	// Each look at the clock is 5ms after the last: the handler is timed with two looks.
	now := time.Unix(1_700_000_000, 0)
	clock := func() time.Time {
		now = now.Add(5 * time.Millisecond)
		return now
	}
	handler := New(Config{Store: meter, MeasureSaves: true, Now: clock})
	cases := []struct{ method, path, operation string }{
		{http.MethodPut, "/api/workspace", "workspace.write"},
		{http.MethodPost, "/api/workspace/records", "record.add"},
		{http.MethodDelete, "/api/workspace/records", "record.delete"},
	}
	for i, c := range cases {
		response := send(handler, c.method, c.path, `{"a":1}`)
		if len(meter.saves) != i+1 {
			t.Fatalf("%s %s: %d saves measured, want %d", c.method, c.path, len(meter.saves), i+1)
		}
		got := meter.saves[i]
		if got.Operation != c.operation || got.Status != response.Code || got.RequestBytes != len(`{"a":1}`) ||
			got.ResponseBytes != response.Body.Len() || got.ResponseBytes == 0 || got.DurationMS < 5 {
			t.Errorf("%s %s: measured %+v, answered %d with %d bytes", c.method, c.path, got, response.Code, response.Body.Len())
		}
	}
}

func TestMeasuredLeavesOtherRequestsAlone(t *testing.T) {
	meter := &meterStore{}
	handler := New(Config{Store: meter, MeasureSaves: true})
	for _, c := range [][2]string{
		{http.MethodGet, "/api/workspace"}, {http.MethodHead, "/api/workspace"}, {http.MethodPost, "/api/workspace"},
		{http.MethodGet, "/api/workspace/records"}, {http.MethodGet, "/api/health"},
	} {
		get(t, handler, c[0], c[1])
	}
	if len(meter.saves) != 0 {
		t.Fatalf("measured %+v", meter.saves)
	}
}

func TestMeasuredOnlyWhenTurnedOn(t *testing.T) {
	meter := &meterStore{}
	send(New(Config{Store: meter}), http.MethodPut, "/api/workspace", `{}`)
	// A Store that cannot keep a measurement is served as before.
	send(New(Config{Store: fakeStore{}, MeasureSaves: true}), http.MethodPut, "/api/workspace", `{}`)
	if len(meter.saves) != 0 {
		t.Fatalf("measured %+v", meter.saves)
	}
}

// A measurement that cannot be kept does not change the answer.
func TestMeasuredFailureKeepsTheAnswer(t *testing.T) {
	plain := send(New(Config{Store: fakeStore{}}), http.MethodPut, "/api/workspace", `{}`)
	meter := &meterStore{fail: errors.New("down")}
	measured := send(New(Config{Store: meter, MeasureSaves: true}), http.MethodPut, "/api/workspace", `{}`)
	if measured.Code != plain.Code || measured.Body.String() != plain.Body.String() || len(meter.saves) != 1 {
		t.Fatalf("answered %d %s, want %d %s; %d saves measured", measured.Code, measured.Body, plain.Code, plain.Body, len(meter.saves))
	}
}
