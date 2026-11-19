package workspace

import (
	"errors"
	"testing"
)

func text(value string) *string { return &value }

// two lists: l1 英単語 with a record, and l2 Fruits.
func two() *Workspace {
	id := "r1"
	return &Workspace{Lists: []List{
		{ID: "l1", Name: "英単語", Words: []Word{{Display: "apple", Input: "apple", Annotation: "りんご"}},
			Records: []Record{{ID: &id, Time: 1, Date: "2026-10-10T00:00:00.000Z"}}, CreatedAt: "2026-10-01T00:00:00.000Z"},
		{ID: "l2", Name: "Fruits", Words: []Word{{Display: "pear", Input: "pear"}}, Records: []Record{}, CreatedAt: "2026-10-02T00:00:00.000Z"},
	}, Devices: []Device{{ID: "d1", Name: "自宅", Layout: "JIS"}}}
}

func wantKey(t *testing.T, err error, key string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Key != key {
		t.Fatalf("err = %v, want %s", err, key)
	}
}

// The cases are shared with tests/strings.test.cjs, which checks them against Node's toLowerCase.
func TestLowerCase(t *testing.T) {
	for _, c := range stringCases(t).LowerCase {
		if got := lowerCase(c.Input); got != c.Want {
			t.Errorf("lowerCase(%q) = %q, want %q", c.Input, got, c.Want)
		}
	}
}
