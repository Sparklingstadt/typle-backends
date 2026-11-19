package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The scenarios of tests/fixtures/workspace-writes.json and record-writes.json, which tests/serverWorkspaceWrite.test.cjs
// and tests/apiRecords.test.cjs run through src/server/workspace.ts against the same kind of database: each request
// here must answer as there and leave the same row and records. (The Go backend recorded them before v2.47.0; the
// links of typle_shared_packs they also hold are not checked, as sharing was removed in v2.49.0.)

func readFixture(t *testing.T, name string, v any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}

// fixtureRow is a typle_workspaces row of a fixture: its jsonb columns and revision, and records_split as split.
type fixtureRow struct {
	Lists     json.RawMessage `json:"lists"`
	Packs     json.RawMessage `json:"packs"`
	Devices   json.RawMessage `json:"devices"`
	Interests json.RawMessage `json:"interests"`
	Revision  int             `json:"revision"`
	Split     bool            `json:"split"`
}

// nullable is a jsonb column's value: nil for JSON null, so that the column is NULL.
func nullable(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return string(raw)
}

// fixtureDatabase is a connection to the test database, or a skip without one.
func fixtureDatabase(t *testing.T) *pgx.Conn {
	t.Helper()
	connection := os.Getenv("TYPLE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("set TYPLE_TEST_DATABASE_URL to a migrated test database")
	}
	conn, err := pgx.Connect(context.Background(), connection)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

func insertRow(t *testing.T, conn *pgx.Conn, owner string, row *fixtureRow) {
	t.Helper()
	if row == nil {
		return
	}
	if _, err := conn.Exec(context.Background(), `INSERT INTO typle_workspaces (owner_id, lists, packs, devices, interests, revision, records_split)
		VALUES ($1, $2::jsonb, $3::jsonb, $4::jsonb, $5::jsonb, $6, $7)`,
		owner, string(row.Lists), nullable(row.Packs), string(row.Devices), nullable(row.Interests), row.Revision, row.Split); err != nil {
		t.Fatal(err)
	}
}

// state is the owner's row and records as the TypeScript tests read them, as JSON values.
func state(t *testing.T, conn *pgx.Conn, owner string) (row, records any) {
	t.Helper()
	var rowText, recordsText string
	if err := conn.QueryRow(context.Background(), `SELECT COALESCE((SELECT jsonb_build_object('lists', lists, 'packs', packs,
		'devices', devices, 'interests', interests, 'revision', revision, 'split', records_split) FROM typle_workspaces
		WHERE owner_id = $1), 'null'::jsonb)::text`, owner).Scan(&rowText); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(context.Background(), `SELECT COALESCE(jsonb_agg(jsonb_build_array(list_id, record_id, position,
		(extract(epoch FROM at) * 1000)::bigint, data) ORDER BY list_id, position), '[]'::jsonb)::text
		FROM typle_records WHERE owner_id = $1`, owner).Scan(&recordsText); err != nil {
		t.Fatal(err)
	}
	return jsonValue(t, []byte(rowText)), jsonValue(t, []byte(recordsText))
}

func jsonValue(t *testing.T, raw []byte) any {
	t.Helper()
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	return value
}

// withAccount is body with its accountId set to owner.
func withAccount(t *testing.T, body json.RawMessage, owner string) string {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	fields["accountId"], _ = json.Marshal(owner)
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestWorkspaceWritesFixture(t *testing.T) {
	var fixture struct {
		Scenarios []struct {
			Name  string `json:"name"`
			Setup *struct {
				Row *fixtureRow `json:"row"`
			} `json:"setup"`
			Steps []struct {
				Body    json.RawMessage `json:"body"`
				Result  string          `json:"result"`
				Row     json.RawMessage `json:"row"`
				Records json.RawMessage `json:"records"`
			} `json:"steps"`
		} `json:"scenarios"`
	}
	readFixture(t, "workspace-writes.json", &fixture)
	for _, scenario := range fixture.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			s, owner := realStore(t)
			conn := fixtureDatabase(t)
			if scenario.Setup != nil {
				insertRow(t, conn, owner, scenario.Setup.Row)
			}
			h := newHarness(t, s, testBase)
			for i, step := range scenario.Steps {
				response := h.put(owner, withAccount(t, step.Body, owner))
				switch step.Result {
				case "ok":
					if response.Code != http.StatusOK {
						t.Fatalf("step %d: %d %s", i+1, response.Code, response.Body)
					}
				case "conflict":
					expectError(t, response, http.StatusConflict, "conflict")
				default:
					expectError(t, response, http.StatusBadRequest, step.Result)
				}
				row, records := state(t, conn, owner)
				if want := jsonValue(t, step.Row); !reflect.DeepEqual(row, want) {
					t.Fatalf("step %d: row = %v\nwant %v", i+1, row, want)
				}
				if want := jsonValue(t, step.Records); !reflect.DeepEqual(records, want) {
					t.Fatalf("step %d: records = %v\nwant %v", i+1, records, want)
				}
			}
		})
	}
}

func TestRecordWritesFixture(t *testing.T) {
	var fixture struct {
		Scenarios []struct {
			Name  string `json:"name"`
			Setup struct {
				Row *fixtureRow `json:"row"`
			} `json:"setup"`
			Steps []struct {
				Kind    string          `json:"kind"`
				Body    json.RawMessage `json:"body"`
				Status  int             `json:"status"`
				Key     string          `json:"key"`
				Record  json.RawMessage `json:"record"`
				Row     json.RawMessage `json:"row"`
				Records json.RawMessage `json:"records"`
			} `json:"steps"`
		} `json:"scenarios"`
	}
	readFixture(t, "record-writes.json", &fixture)
	for _, scenario := range fixture.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			s, owner := realStore(t)
			conn := fixtureDatabase(t)
			insertRow(t, conn, owner, scenario.Setup.Row)
			h := newHarness(t, s, testBase)
			for i, step := range scenario.Steps {
				var change []func(*http.Request)
				if step.Kind == "delete" {
					change = append(change, func(r *http.Request) { r.Method = http.MethodDelete })
				}
				response := h.postRecord(owner, withAccount(t, step.Body, owner), change...)
				if response.Code != step.Status {
					t.Fatalf("step %d: %d %s; want %d", i+1, response.Code, response.Body, step.Status)
				}
				if step.Key != "" {
					expectError(t, response, step.Status, step.Key)
				}
				if step.Kind == "add" && step.Key == "" {
					got := decodeBody(t, response)["record"]
					if !reflect.DeepEqual(jsonValue(t, got), jsonValue(t, step.Record)) {
						t.Fatalf("step %d: record = %s; want %s", i+1, got, step.Record)
					}
				}
				row, records := state(t, conn, owner)
				if want := jsonValue(t, step.Row); !reflect.DeepEqual(row, want) {
					t.Fatalf("step %d: row = %v\nwant %v", i+1, row, want)
				}
				if want := jsonValue(t, step.Records); !reflect.DeepEqual(records, want) {
					t.Fatalf("step %d: records = %v\nwant %v", i+1, records, want)
				}
			}
		})
	}
}

// The tables of sharing, the gallery and the dashboard were dropped (migrations 0036 to 0038), and nothing here
// touches them; the saves above pass without them.
func TestDroppedTablesAreGone(t *testing.T) {
	conn := fixtureDatabase(t)
	for _, table := range []string{"typle_shared_packs", "typle_gallery_reports", "typle_gallery_hits", "typle_admins"} {
		var gone bool
		if err := conn.QueryRow(context.Background(), `SELECT to_regclass($1) IS NULL`, table).Scan(&gone); err != nil || !gone {
			t.Errorf("%s: gone %v, %v", table, gone, err)
		}
	}
}

// A save that changes nothing still rewrites the row and moves the revision on, as writeWorkspace does, so that the
// benchmark's PUT of what GET read is a real write.
func TestUnchangedSaveBumpsRevision(t *testing.T) {
	s, owner := realStore(t)
	h := newHarness(t, s, testBase)
	body := `"packs":[{"id":"p","name":"P","createdAt":"2026-09-06"}],"lists":[` +
		`{"id":"one","name":"One","packId":"p","createdAt":"2026-09-06","words":[{"display":"a","input":"a","annotation":""}],"records":[]}]`
	for revision := range 3 {
		response := h.put(owner, payload(owner, `"revision":`+strconv.Itoa(revision)+`,`+body))
		var answer struct{ Revision int }
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil || response.Code != http.StatusOK || answer.Revision != revision+1 {
			t.Fatalf("save at %d = %d %s", revision, response.Code, response.Body)
		}
	}
}
