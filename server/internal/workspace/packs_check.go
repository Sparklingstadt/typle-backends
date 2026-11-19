package workspace

import (
	"encoding/json"
	"slices"
)

// CheckPacks follows validateLibrary (src/lib/wordPacks.ts): it refuses packs and lists that are not in the saved
// shape, with invalidWorkspace, and lists of a pack whose paths overlap, with levelsConflict. Unlike validateLibrary it
// takes a list with several levels (up to MaxSavedLevels, see CheckLimits): apps before v0.97.0, which may still be
// open or not yet updated, send them with their packs, and a save they cannot change must not fail. Such a list is
// stored as Decode repairs it, joined into one level (collapseLevels) and told apart (separateLevels), so it is not
// checked for overlaps here.
func CheckPacks(packs []Pack, lists []List) error {
	invalid := &Error{Key: "invalidWorkspace"}
	ids := map[string]bool{}
	names := map[string]bool{}
	for _, pack := range packs {
		name := nameKey(pack.Name)
		if pack.ID == "" || ids[pack.ID] || name == "" || length(pack.Name) > MaxPackName || names[name] ||
			length(pack.Description) > MaxPackDescription || length(pack.Category) > MaxCategory || len(pack.Tags) > MaxTags ||
			slices.ContainsFunc(pack.Tags, func(tag string) bool { return length(tag) > MaxTag }) {
			return invalid
		}
		ids[pack.ID] = true
		names[name] = true
	}
	listNames := map[string]map[string]bool{}
	for _, list := range lists {
		taken := listNames[list.PackID]
		if taken == nil {
			taken = map[string]bool{}
			listNames[list.PackID] = taken
		}
		name := nameKey(list.Name)
		// CheckLimits sees the lists as Decode joined their levels, so the number sent is checked here.
		if list.PackID == "" || !ids[list.PackID] || taken[name] || list.Group != "" || list.Category != "" || list.Tags != nil ||
			len(list.Levels) > MaxSavedLevels {
			return invalid
		}
		taken[name] = true
	}
	if len(ids) != len(listNames) {
		return invalid
	}
	// Lists of a pack whose paths overlap are refused as checkReach refuses them; separateLevels repairs saved ones.
	paths := map[string][][]string{}
	for _, list := range lists {
		if len(list.Levels) > MaxLevels {
			continue
		}
		path := levelPath(list.Name, list.Levels)
		if slices.ContainsFunc(paths[list.PackID], func(other []string) bool { return pathsOverlap(other, path) }) {
			return &Error{Key: "levelsConflict"}
		}
		paths[list.PackID] = append(paths[list.PackID], path)
	}
	return nil
}

// CheckLibrary follows validateLibrary (src/lib/wordPacks.ts) for a save that sends lists and packs. It reads them as
// they were sent, before Decode repairs them: a list may not have a group, category or tags at all, not even "" or [],
// and a pack's description, category, tags and origin may be left out but not null, as decodePacks requires. Anything
// refused is invalidWorkspace, but for lists whose levels overlap (levelsConflict, see CheckPacks).
func CheckLibrary(lists, packs []byte) error {
	invalid := &Error{Key: "invalidWorkspace"}
	decoded, err := DecodeLists(lists)
	if err != nil || has(lists, func(key string, _ json.RawMessage) bool { return key == "group" || key == "category" || key == "tags" }) ||
		has(packs, func(key string, value json.RawMessage) bool {
			return (key == "description" || key == "category" || key == "tags" || key == "origin") && string(value) == "null"
		}) {
		return invalid
	}
	saved, err := decodePacks(packs)
	if err != nil {
		return invalid
	}
	return CheckPacks(saved, decoded)
}

// has reports whether an object of the JSON array data has a member that found accepts. data that is not an array of
// objects has none; reading it is left to the decoders.
func has(data []byte, found func(key string, value json.RawMessage) bool) bool {
	var items []map[string]json.RawMessage
	if json.Unmarshal(data, &items) != nil {
		return false
	}
	for _, item := range items {
		for key, value := range item {
			if found(key, value) {
				return true
			}
		}
	}
	return false
}

// levelPath follows levelPath (src/lib/packEditing.ts): the levels the practice screen picks a list of a pack by, or
// its name when it has none.
func levelPath(name string, levels []string) []string {
	if len(levels) > 0 {
		return levels
	}
	return []string{name}
}

// pathsOverlap follows pathsOverlap (src/lib/packEditing.ts): two paths that are equal, or one the start of the other,
// leave one of their lists out of reach.
func pathsOverlap(a, b []string) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	return slices.Equal(a, b[:len(a)])
}
