package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// Word packs follow src/core/wordPacks.ts: every list belongs to one pack, which holds the category and tags the lists
// used to have. The rules here are the app's, so both read the same saved data into the same packs; the fixtures in
// tests/fixtures/packs are checked by both.

// The limits of a pack (MAX_PACK_NAME and MAX_PACK_DESCRIPTION in src/core/wordPacks.ts), in UTF-16 code units.
const (
	MaxPackName        = 200
	MaxPackDescription = 500
)

// Origin is set on a pack that came from the official catalog. Without Followed it is a copy from before v0.98.0, which
// Decode removes (see followOfficial).
type Origin struct {
	CatalogID string  `json:"catalogId"`
	Version   float64 `json:"version"`
	Followed  bool    `json:"followed,omitempty"`
}

// Pack is a word pack. Description and Category are "" and Tags nil when it has none, which is how they are stored:
// left out.
type Pack struct {
	ID          string
	Name        string
	Description string
	Category    string
	Tags        []string
	Origin      *Origin
	CreatedAt   string
	// Extra holds the stored fields Typle does not know (see extra.go).
	Extra string
}

type rawOrigin struct {
	CatalogID *string  `json:"catalogId"`
	Version   *float64 `json:"version"`
	Followed  *bool    `json:"followed"`
}

type rawPack struct {
	ID          *string    `json:"id"`
	Name        *string    `json:"name"`
	Description *string    `json:"description"`
	Category    *string    `json:"category"`
	Tags        []*string  `json:"tags"`
	Origin      *rawOrigin `json:"origin"`
	CreatedAt   *string    `json:"createdAt"`
	extra       string
}

func (r *rawPack) UnmarshalJSON(data []byte) (err error) {
	type plain rawPack
	if err = json.Unmarshal(data, (*plain)(r)); err == nil {
		r.extra, err = unknown(data, "id", "name", "description", "category", "tags", "origin", "createdAt")
	}
	return err
}

// The stored shape of a pack.
type storedPack struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Origin      *Origin  `json:"origin,omitempty"`
	CreatedAt   string   `json:"createdAt"`
	extra       string
}

func (p storedPack) MarshalJSON() ([]byte, error) {
	type plain storedPack
	known, err := marshal(plain(p))
	return withExtra(known, p.extra), err
}

// EncodePacks writes packs as the app saves them in the packs column, which its decodePacks reads back.
func EncodePacks(packs []Pack) ([]byte, error) {
	stored := make([]storedPack, 0, len(packs))
	for _, pack := range packs {
		stored = append(stored, storedPack{ID: pack.ID, Name: pack.Name, Description: pack.Description, Category: pack.Category,
			Tags: pack.Tags, Origin: pack.Origin, CreatedAt: pack.CreatedAt, extra: pack.Extra})
	}
	return marshal(stored)
}

// decodePacks follows decodePacks (src/core/wordPacks.ts).
func decodePacks(data []byte) ([]Pack, error) {
	var raw []*rawPack
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("packs are not valid: %w", err)
	}
	if raw == nil {
		return nil, errors.New("packs must be an array")
	}
	packs := make([]Pack, 0, len(raw))
	ids := map[string]bool{}
	for index, item := range raw {
		if item == nil || item.ID == nil || *item.ID == "" || ids[*item.ID] || item.Name == nil || item.CreatedAt == nil ||
			(item.Origin != nil && (item.Origin.CatalogID == nil || item.Origin.Version == nil ||
				(item.Origin.Followed != nil && !*item.Origin.Followed))) {
			return nil, fmt.Errorf("pack %d is invalid", index+1)
		}
		ids[*item.ID] = true
		pack := Pack{ID: *item.ID, Name: *item.Name, CreatedAt: *item.CreatedAt, Extra: item.extra}
		if item.Description != nil {
			pack.Description = *item.Description
		}
		if item.Category != nil {
			pack.Category = *item.Category
		}
		for _, tag := range item.Tags {
			if tag == nil {
				return nil, fmt.Errorf("pack %d: tags must be strings", index+1)
			}
			pack.Tags = append(pack.Tags, *tag)
		}
		if item.Origin != nil {
			pack.Origin = &Origin{CatalogID: *item.Origin.CatalogID, Version: *item.Origin.Version, Followed: item.Origin.Followed != nil}
		}
		packs = append(packs, pack)
	}
	return packs, nil
}

// followed follows isFollowed (src/core/followedPacks.ts): the pack is a followed official pack.
func (p *Pack) followed() bool { return p.Origin != nil && p.Origin.Followed }

func followedIDs(packs []Pack) map[string]bool {
	ids := map[string]bool{}
	for i := range packs {
		if packs[i].followed() {
			ids[packs[i].ID] = true
		}
	}
	return ids
}

// OwnListCount is ownListCount (src/core/listOverview.ts): the lists MaxLists counts, those not in a followed pack,
// whose words the catalog has.
func OwnListCount(packs []Pack, lists []List) int {
	followed := followedIDs(packs)
	count := 0
	for _, list := range lists {
		if !followed[list.PackID] {
			count++
		}
	}
	return count
}

// FollowedPackCount is followedPackCount (src/core/listOverview.ts).
func FollowedPackCount(packs []Pack) int { return len(followedIDs(packs)) }

// viewLists follows viewLists (src/core/wordPacks.ts): each list carries its pack's category and tags, so the code that
// reads them from lists sees what the app's screens see. A list's group is not given: Go does not show it.
func viewLists(w *Workspace) {
	byID := map[string]*Pack{}
	for i := range w.Packs {
		byID[w.Packs[i].ID] = &w.Packs[i]
	}
	for i := range w.Lists {
		if pack := byID[w.Lists[i].PackID]; pack != nil {
			w.Lists[i].Category, w.Lists[i].Tags = pack.Category, slices.Clone(pack.Tags)
		}
	}
}

// ApplyListChange follows applyListChange (src/core/wordPacks.ts): it puts a change made to the lists of previous (the
// workspace as Decode read it) back into the packs. The lists then carry their packs' labels again.
//
// The packs are those of w, as applyListChange takes them.
func (w *Workspace) ApplyListChange(previous *Workspace) {
	before := map[string]List{}
	for _, list := range previous.Lists {
		before[list.ID] = list
	}
	names := map[string]string{}
	for _, pack := range previous.Packs {
		names[pack.ID] = pack.Name
	}
	packs := make([]Pack, 0, len(w.Packs))
	for _, pack := range w.Packs {
		packs = append(packs, changedPack(pack, w.Lists, previous.Lists, before, names))
	}
	// 3. Labels of lists in an existing pack were settled in 1, so they are dropped; only a list without a pack keeps
	// them for repair to move.
	known := map[string]bool{}
	for _, pack := range packs {
		known[pack.ID] = true
	}
	lists := make([]List, 0, len(w.Lists))
	for _, list := range w.Lists {
		if list.PackID != "" && known[list.PackID] {
			list = withoutLabels(list)
		}
		lists = append(lists, list)
	}
	w.Packs, w.Lists = repairLibrary(packs, lists)
	viewLists(w)
}

// changedPack is steps 1 and 2 of ApplyListChange for pack: lists are the lists after the change and previous those
// before it (before by id), and names the packs' names before it.
func changedPack(pack Pack, lists, previous []List, before map[string]List, names map[string]string) Pack {
	mine, only := listsIn(lists, pack.ID), listsIn(previous, pack.ID)
	next := pack
	next.Tags = slices.Clone(pack.Tags)
	// 1. The first list whose labels differ from the pack's gives the pack its new labels.
	wasCategory, wasTags := labelsOf(pack.Category, pack.Tags)
	for _, list := range mine {
		category, tags := labelsOf(list.Category, list.Tags)
		if category != wasCategory || !slices.Equal(tags, wasTags) {
			next.Category, next.Tags = category, tags
			break
		}
	}
	// 2. A pack of one list follows that list's rename, unless the change renamed the pack itself.
	if name, found := names[pack.ID]; found && name == pack.Name && len(only) == 1 && len(mine) == 1 && mine[0].ID == only[0].ID &&
		nameKey(pack.Name) == nameKey(before[mine[0].ID].Name) {
		next.Name = mine[0].Name
	}
	return next
}
