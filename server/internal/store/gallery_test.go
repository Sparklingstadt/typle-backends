package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sparklingstadt/typle-r/server/internal/workspace"
)

// galleryRow inserts a share of the owner whose snapshot has the given pack fields and lists of n words each. listed
// and hidden are the times to set, or nil.
type galleryRow struct {
	pack           map[string]any
	words          []int
	listed, hidden *time.Time
}

func insertShare(t *testing.T, conn *pgx.Conn, owner string, n int, row galleryRow) string {
	t.Helper()
	lists := []map[string]any{}
	for i, count := range row.words {
		words := make([]map[string]any, count)
		for j := range words {
			words[j] = map[string]any{"display": fmt.Sprint(j)}
		}
		lists = append(lists, map[string]any{"name": fmt.Sprint("list", i), "words": words})
	}
	data, err := json.Marshal(map[string]any{"format": "typle-pack", "version": 1, "pack": row.pack, "lists": lists})
	if err != nil {
		t.Fatal(err)
	}
	id := workspace.NewID()
	if _, err := conn.Exec(context.Background(), `INSERT INTO typle_shared_packs (id, owner_id, pack_id, data, listed_at, hidden_at)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6)`, id, owner, fmt.Sprint("p", n), string(data), row.listed, row.hidden); err != nil {
		t.Fatal(err)
	}
	return id
}

func listedIn(offset time.Duration) *time.Time {
	at := time.Now().Add(offset)
	return &at
}

func reportRows(t *testing.T, conn *pgx.Conn, share string) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(context.Background(), `SELECT count(*)::int FROM typle_gallery_reports WHERE share_id = $1`, share).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestReportsGoWithShare(t *testing.T) {
	ctx := context.Background()
	connection, conn := database(t)
	id := owner(t, conn)
	s := open(t, connection)
	w := saved(t, s, id)
	pack := w.Packs[0].ID
	yes := true
	share, err := s.PutShare(ctx, id, pack, &yes)
	if err != nil {
		t.Fatal(err)
	}
	// Next.js records the report (src/server/galleryWrites.ts); what matters here is that it goes with the share.
	if _, err := conn.Exec(ctx, `INSERT INTO typle_gallery_reports (id, share_id, reason) VALUES ($1, $2, 'other')`, workspace.NewID(), share.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteShare(ctx, id, pack); err != nil {
		t.Fatal(err)
	}
	if n := reportRows(t, conn, share.ID); n != 0 {
		t.Errorf("rows = %d, want 0", n)
	}
}

func TestAdminGallery(t *testing.T) {
	ctx := context.Background()
	connection, conn := database(t)
	id := owner(t, conn)
	s := open(t, connection)
	category := "管理" + id
	pack := func(name string) map[string]any { return map[string]any{"name": name, "category": category} }
	// Every share the test looks for has a report, so it ranks above the shares without one that other tests leave in
	// the shared database and cannot be pushed out of the first AdminGalleryRows.
	many := insertShare(t, conn, id, 1, galleryRow{pack: pack("many"), listed: listedIn(-time.Hour)})
	newer := insertShare(t, conn, id, 2, galleryRow{pack: pack("newer"), listed: listedIn(-5 * time.Hour)})
	older := insertShare(t, conn, id, 3, galleryRow{pack: pack("older"), listed: listedIn(-4 * time.Hour)})
	listedNew := insertShare(t, conn, id, 4, galleryRow{pack: pack("listedNew"), listed: listedIn(-2 * time.Hour)})
	hidden := insertShare(t, conn, id, 5, galleryRow{pack: pack("hidden"), listed: listedIn(-3 * time.Hour), hidden: listedIn(0)})
	// Hiding a share that was never listed still shows it, after the listed ones.
	hiddenOnly := insertShare(t, conn, id, 6, galleryRow{pack: pack("hiddenOnly"), hidden: listedIn(0)})
	linkOnly := insertShare(t, conn, id, 7, galleryRow{pack: pack("linkOnly")})

	// Reports are inserted directly (a report of a hidden share is refused), each at a fixed time: three for many, two for newer
	// and older (newer has the more recent last report), and one each for the rest at the same time, so that only
	// listed_at (the newest first, a share never listed last) tells them apart.
	base := time.Now().Add(-time.Hour)
	for _, r := range []struct {
		share, reason string
		at            time.Time
	}{
		{many, "spam", base.Add(-30 * time.Minute)}, {many, "spam", base.Add(-20 * time.Minute)}, {many, "inappropriate", base.Add(-10 * time.Minute)},
		{newer, "other", base.Add(-40 * time.Minute)}, {newer, "spam", base.Add(-5 * time.Minute)},
		{older, "other", base.Add(-50 * time.Minute)}, {older, "spam", base.Add(-6 * time.Minute)},
		{listedNew, "other", base}, {hidden, "personalInfo", base}, {hiddenOnly, "other", base},
	} {
		if _, err := conn.Exec(ctx, `INSERT INTO typle_gallery_reports (id, share_id, reason, created_at) VALUES ($1, $2, $3, $4)`,
			workspace.NewID(), r.share, r.reason, r.at); err != nil {
			t.Fatal(err)
		}
	}

	packs, err := s.AdminGallery(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) > AdminGalleryRows {
		t.Errorf("rows = %d, want at most %d", len(packs), AdminGalleryRows)
	}
	index := map[string]int{}
	byID := map[string]AdminGalleryPack{}
	for i, p := range packs {
		index[p.ID] = i
		byID[p.ID] = p
	}
	order := []string{many, newer, older, listedNew, hidden, hiddenOnly}
	for i, share := range order {
		if _, ok := byID[share]; !ok {
			t.Fatalf("%s missing from the admin gallery", share)
		}
		if i > 0 && index[order[i-1]] >= index[share] {
			t.Errorf("order: %s (%d) should come before %s (%d)", order[i-1], index[order[i-1]], share, index[share])
		}
	}
	if _, ok := byID[linkOnly]; ok {
		t.Error("a link-only share is in the admin gallery")
	}
	got := byID[many]
	if got.Reports != 3 || got.Reasons["spam"] != 2 || got.Reasons["inappropriate"] != 1 || len(got.Reasons) != 2 ||
		got.Name != "many" || got.Category != category || got.ListedAt == nil || got.HiddenAt != nil {
		t.Errorf("many = %+v", got)
	}
	if h := byID[hidden]; h.HiddenAt == nil || h.ListedAt == nil || h.Reports != 1 || h.Reasons["personalInfo"] != 1 {
		t.Errorf("hidden = %+v", h)
	}
	if h := byID[hiddenOnly]; h.HiddenAt == nil || h.ListedAt != nil {
		t.Errorf("hiddenOnly = %+v", h)
	}
}

func hiddenAt(t *testing.T, conn *pgx.Conn, id string) *time.Time {
	t.Helper()
	var at *time.Time
	if err := conn.QueryRow(context.Background(), `SELECT hidden_at FROM typle_shared_packs WHERE id = $1`, id).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

func TestSetHidden(t *testing.T) {
	ctx := context.Background()
	connection, conn := database(t)
	id := owner(t, conn)
	s := open(t, connection)
	w := saved(t, s, id)
	name := "非公開" + id
	if _, err := s.Modify(ctx, id, func(w *workspace.Workspace) (bool, error) {
		w.FindPack(w.Packs[0].ID).Name = name
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	yes := true
	shared, err := s.PutShare(ctx, id, w.Packs[0].ID, &yes)
	if err != nil {
		t.Fatal(err)
	}
	share := shared.ID

	if ok, err := s.SetHidden(ctx, share, true); err != nil || !ok {
		t.Fatalf("hide = %v, %v", ok, err)
	}
	first := hiddenAt(t, conn, share)
	if first == nil {
		t.Fatal("hidden_at not set")
	}
	time.Sleep(10 * time.Millisecond)
	if ok, err := s.SetHidden(ctx, share, true); err != nil || !ok {
		t.Fatalf("hide again = %v, %v", ok, err)
	}
	if again := hiddenAt(t, conn, share); again == nil || !again.Equal(*first) {
		t.Errorf("hidden_at = %v, want %v", again, first)
	}
	if ok, err := s.SetHidden(ctx, share, false); err != nil || !ok {
		t.Fatalf("unhide = %v, %v", ok, err)
	}
	if hiddenAt(t, conn, share) != nil {
		t.Error("hidden_at not cleared")
	}
	for _, missing := range []string{workspace.NewID(), "x"} {
		if ok, err := s.SetHidden(ctx, missing, true); err != nil || ok {
			t.Errorf("SetHidden(%q) = %v, %v; want false, nil", missing, ok, err)
		}
	}
}

func TestSetHiddenKeepsListing(t *testing.T) {
	ctx := context.Background()
	connection, conn := database(t)
	id := owner(t, conn)
	s := open(t, connection)
	category := "掲載維持" + id
	share := insertShare(t, conn, id, 1, galleryRow{pack: map[string]any{"name": "a", "category": category}, listed: listedIn(-time.Hour)})
	before := listedAt(t, conn, share)
	for _, hide := range []bool{true, false} {
		if ok, err := s.SetHidden(ctx, share, hide); err != nil || !ok {
			t.Fatalf("SetHidden(%v) = %v, %v", hide, ok, err)
		}
		if at := listedAt(t, conn, share); at == nil || !at.Equal(*before) {
			t.Fatalf("listed_at = %v, want %v", at, before)
		}
	}
}
