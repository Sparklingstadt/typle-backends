package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sparklingstadt/typle-r/server/internal/workspace"
)

// The gallery is the shares the owners list (listed_at) and the operator has not hidden (hidden_at). Next.js reads it
// (src/server/gallery.ts); here are its counts, its reports and the administrator's part.

// CountAdded counts one more addition of the listed pack id to someone's packs. A pack that is not listed, is hidden,
// does not exist or has an id that is not a UUID is left as it is, without an error.
func (s *Store) CountAdded(ctx context.Context, id string) error {
	if !isUUID(id) {
		return nil
	}
	_, err := s.pool.Exec(ctx, `UPDATE typle_shared_packs SET add_count = add_count + 1
		WHERE id = $1 AND listed_at IS NOT NULL AND hidden_at IS NULL`, id)
	return err
}

// ReportReasons are the reasons a gallery pack can be reported for.
var ReportReasons = []string{"inappropriate", "spam", "personalInfo", "other"}

// Report records a report of a listed, unhidden share. It returns false when there is no such share (or id is not a
// UUID); reason is checked by the caller.
func (s *Store) Report(ctx context.Context, id, reason string) (bool, error) {
	if !isUUID(id) {
		return false, nil
	}
	tag, err := s.pool.Exec(ctx, `INSERT INTO typle_gallery_reports (id, share_id, reason)
		SELECT $1, id, $3 FROM typle_shared_packs WHERE id = $2 AND listed_at IS NOT NULL AND hidden_at IS NULL`,
		workspace.NewID(), id, reason)
	// The share can be deleted between the statement's snapshot and the foreign key check; it is gone then.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// AdminGalleryPack is a listed or hidden pack as the administrator sees it, with its reports.
type AdminGalleryPack struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Category string         `json:"category,omitempty"`
	Added    int            `json:"added"`
	ListedAt *time.Time     `json:"listedAt"`
	HiddenAt *time.Time     `json:"hiddenAt"`
	Reports  int            `json:"reports"`
	Reasons  map[string]int `json:"reasons"`
}

// AdminGalleryRows is the number of packs AdminGallery returns at most.
const AdminGalleryRows = 100

// AdminGallery lists listed or hidden shares, the most reported first, then the most recent report, then listed_at
// (the newest first).
func (s *Store) AdminGallery(ctx context.Context) ([]AdminGalleryPack, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.id::text, COALESCE(p.data #>> '{pack,name}', ''),
			COALESCE(p.data #>> '{pack,category}', ''), p.add_count, p.listed_at, p.hidden_at,
			COALESCE(r.reports, 0), COALESCE(r.reasons, '{}'::jsonb)::text
		FROM typle_shared_packs p
		LEFT JOIN (SELECT share_id, sum(n)::int AS reports, max(last) AS last, jsonb_object_agg(reason, n) AS reasons
			FROM (SELECT share_id, reason, count(*) AS n, max(created_at) AS last
				FROM typle_gallery_reports GROUP BY share_id, reason) AS by_reason
			GROUP BY share_id) AS r ON r.share_id = p.id
		WHERE p.listed_at IS NOT NULL OR p.hidden_at IS NOT NULL
		ORDER BY COALESCE(r.reports, 0) DESC, r.last DESC NULLS LAST, p.listed_at DESC NULLS LAST, p.id
		LIMIT $1`, AdminGalleryRows)
	if err != nil {
		return nil, err
	}
	packs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AdminGalleryPack, error) {
		var pack AdminGalleryPack
		var reasons string
		if err := row.Scan(&pack.ID, &pack.Name, &pack.Category, &pack.Added, &pack.ListedAt, &pack.HiddenAt,
			&pack.Reports, &reasons); err != nil {
			return pack, err
		}
		pack.Reasons = map[string]int{}
		return pack, json.Unmarshal([]byte(reasons), &pack.Reasons)
	})
	if err != nil {
		return nil, err
	}
	return append([]AdminGalleryPack{}, packs...), nil
}

// SetHidden sets or clears hidden_at, leaving listed_at as it is. Hiding a hidden share keeps its first time. It
// returns false when there is no share with the id.
func (s *Store) SetHidden(ctx context.Context, id string, hidden bool) (bool, error) {
	if !isUUID(id) {
		return false, nil
	}
	tag, err := s.pool.Exec(ctx, `UPDATE typle_shared_packs
		SET hidden_at = CASE WHEN $2::boolean THEN COALESCE(hidden_at, now()) END WHERE id = $1`, id, hidden)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
