package workspace

import "slices"

// Converting and repairing the library of older apps follows src/core/wordPacks.ts: names made unique, levels
// collapsed or separated, labels moved from lists to their packs.

// normalizeLabels follows normalizeLabels (src/core/wordLists.ts): the category and tags trimmed, without empty ones or
// a tag repeated in another case; nil when there are no tags.
func normalizeLabels(category string, tags []string) (string, []string) {
	var kept []string
	seen := map[string]bool{}
	for _, tag := range tags {
		value := trimJS(tag)
		if value == "" || seen[lowerCase(value)] {
			continue
		}
		seen[lowerCase(value)] = true
		kept = append(kept, value)
	}
	return trimJS(category), kept
}

// labelsOf follows labelsOf (src/core/wordPacks.ts): the tags of a group may add up to more than a pack holds, so the
// first MaxTags stay.
func labelsOf(category string, tags []string) (string, []string) {
	category, kept := normalizeLabels(category, tags)
	if len(kept) > MaxTags {
		kept = kept[:MaxTags]
	}
	return category, kept
}

func withoutLabels(list List) List {
	list.Group, list.Category, list.Tags = "", "", nil
	return list
}

// earlier tells whether the date a is before b, as Date.parse(a) < Date.parse(b) does: a date that does not parse is
// never before or after another.
func earlier(a, b string) bool {
	at, okA := ParseTime(a)
	bt, okB := ParseTime(b)
	return okA && okB && at.Before(bt)
}

// convertLibrary follows convertLibrary (src/core/wordPacks.ts): one pack for each group, named after it, and one for
// each list without a group, named after the list. The catalog's lists (src/core/packs) are written so.
func convertLibrary(lists []List) ([]Pack, []List) {
	packs := []Pack{}
	packOf := map[string]int{}
	names := map[string]bool{}
	converted := make([]List, 0, len(lists))
	for _, list := range lists {
		group := trimJS(list.Group)
		key := "list:" + list.ID
		if group != "" {
			key = "group:" + group
		}
		at, found := packOf[key]
		if !found {
			members := []List{list}
			if group != "" {
				members = slices.DeleteFunc(slices.Clone(lists), func(item List) bool { return trimJS(item.Group) != group })
			}
			oldest := members[0]
			var tags []string
			for _, member := range members {
				if earlier(member.CreatedAt, oldest.CreatedAt) {
					oldest = member
				}
				tags = append(tags, member.Tags...)
			}
			wanted := group
			if wanted == "" {
				wanted = trimJS(list.Name)
			}
			category, kept := labelsOf(list.Category, tags)
			packs = append(packs, Pack{ID: "pack-" + list.ID, Name: unique(names, wanted, numbered(wanted)), Category: category, Tags: kept,
				CreatedAt: oldest.CreatedAt})
			at = len(packs) - 1
			packOf[key] = at
		}
		list = withoutLabels(list)
		list.PackID = packs[at].ID
		converted = append(converted, list)
	}
	return packs, separateLevels(converted)
}

// repairLibrary follows repairLibrary (src/core/wordPacks.ts), which keeps the rules of ownership for data an older app
// saved: every list in one pack, no empty packs, no repeated names.
func repairLibrary(saved []Pack, lists []List) ([]Pack, []List) {
	packs := make([]Pack, 0, len(saved))
	byID := map[string]int{}
	for _, pack := range saved {
		pack.Tags = slices.Clone(pack.Tags)
		byID[pack.ID] = len(packs)
		packs = append(packs, pack)
	}
	// A list without a valid pack gets one of its own, or joins the one a former conversion made for it.
	placed := slices.Clone(lists)
	for i, list := range placed {
		if _, ok := byID[list.PackID]; ok && list.PackID != "" {
			continue
		}
		id := "pack-" + list.ID
		if _, ok := byID[id]; !ok {
			byID[id] = len(packs)
			packs = append(packs, Pack{ID: id, Name: trimJS(list.Name), CreatedAt: list.CreatedAt})
		}
		placed[i].PackID = id
	}
	// Labels an older app left on a list move to the pack only while the pack has none.
	used := map[string]bool{}
	for i, list := range placed {
		pack := &packs[byID[list.PackID]]
		if pack.Category == "" && len(pack.Tags) == 0 {
			pack.Category, pack.Tags = normalizeLabels(list.Category, list.Tags)
		}
		placed[i] = withoutLabels(list)
		used[list.PackID] = true
	}
	return uniquePackNames(packs, used), separateLevels(uniqueListNames(placed))
}
