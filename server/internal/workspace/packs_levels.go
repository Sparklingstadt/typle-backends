package workspace

import (
	"fmt"
	"slices"
	"strings"
)

// Levels follow src/core/wordPacks.ts: lists of apps before v0.97.0 may have several, which are collapsed into one
// or separated so that no two lists of a pack overlap.

// bare follows bare (src/core/wordPacks.ts): a level without the spaces and ・ at its ends.
func bare(level string) string {
	return strings.TrimFunc(level, func(r rune) bool { return r == '・' || spaceJS(r) })
}

// collapseLevels follows collapseLevels (src/core/wordPacks.ts): a list holds one level at most since v0.97.0, so
// several levels saved by an older app become one: the levels made bare, without empty ones, joined with ・, cut to
// MaxLevel and made bare again; no level at all when none is left.
func collapseLevels(list List) List {
	if len(list.Levels) <= 1 {
		return list
	}
	var kept []string
	for _, level := range list.Levels {
		if level = bare(level); level != "" {
			kept = append(kept, level)
		}
	}
	list.Levels = nil
	if joined := bare(fit(strings.Join(kept, "・"), "", MaxLevel)); joined != "" {
		list.Levels = []string{joined}
	}
	return list
}

// separateLevels follows separateLevels (src/core/wordPacks.ts), which brings back in reach the lists an older app let
// overlap (see pathsOverlap): a list whose path equals that of an earlier list of its pack gets （2）, （3）… on its
// level, or on its name when it has no levels (a name another list of the pack does not use). Several levels of a list
// are joined into one first (collapseLevels). Earlier lists, and data without overlaps, stay as they are.
func separateLevels(lists []List) []List {
	separated := make([]List, len(lists))
	for i, list := range lists {
		separated[i] = collapseLevels(list)
	}
	names := map[string]map[string]bool{}
	for _, list := range separated {
		if names[list.PackID] == nil {
			names[list.PackID] = map[string]bool{}
		}
		names[list.PackID][nameKey(list.Name)] = true
	}
	accepted := map[string][][]string{}
	for i, list := range separated {
		taken := names[list.PackID]
		hasLevels := len(list.Levels) > 0
		limit := MaxListName
		if hasLevels {
			limit = MaxLevel
		}
		base := levelPath(list.Name, list.Levels)
		path := slices.Clone(base)
		nameTaken := func() bool { return !hasLevels && nameKey(path[0]) != nameKey(list.Name) && taken[nameKey(path[0])] }
		overlaps := func() bool {
			return slices.ContainsFunc(accepted[list.PackID], func(q []string) bool { return pathsOverlap(q, path) })
		}
		for count := 2; overlaps() || nameTaken(); count++ {
			path[0] = fit(base[0], fmt.Sprintf("（%d）", count), limit)
		}
		accepted[list.PackID] = append(accepted[list.PackID], path)
		if path[0] == base[0] {
			continue
		}
		if hasLevels {
			separated[i].Levels = path
		} else {
			separated[i].Name = path[0]
			taken[nameKey(path[0])] = true
		}
	}
	return separated
}
