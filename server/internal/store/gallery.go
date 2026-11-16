package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// The gallery is the shares the owners list (listed_at) and the operator has not hidden (hidden_at). Next.js reads it
// (src/server/gallery.ts); here are its counts, its reports and the administrator's part.

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
