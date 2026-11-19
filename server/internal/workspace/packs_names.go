package workspace

import (
	"fmt"
	"unicode/utf16"
)

// Names follow src/lib/workspaceDocument.ts and src/lib/wordPacks.ts: compared without case, and made unique with
// a number when they are taken.

func nameKey(name string) string { return lowerCase(trimJS(name)) }

// unique follows unique (src/lib/workspaceDocument.ts): wanted, or the first variant whose lowercase is not taken.
func unique(taken map[string]bool, wanted string, variant func(count int) string) string {
	value := wanted
	for count := 2; taken[lowerCase(value)]; count++ {
		value = variant(count)
	}
	taken[lowerCase(value)] = true
	return value
}

// fit follows fit (src/lib/workspaceDocument.ts): name with suffix after it, the name cut so that the whole stays
// within limit UTF-16 code units, without the spaces the cut leaves at its end. A character of two code units is not
// split.
func fit(name, suffix string, limit int) string {
	units := utf16.Encode([]rune(name))
	if keep := limit - length(suffix); len(units) > keep {
		units = units[:max(keep, 0)]
		if n := len(units); n > 0 && units[n-1] >= 0xD800 && units[n-1] <= 0xDBFF {
			units = units[:n-1]
		}
	}
	return trimEndJS(string(utf16.Decode(units))) + suffix
}

// numbered gives the variants 名前（2）, 名前（3）…, cut to stay within MaxListName.
func numbered(name string) func(count int) string {
	return func(count int) string { return fit(name, fmt.Sprintf("（%d）", count), MaxListName) }
}

// uniquePackNames keeps the packs that are used, trimming their names and numbering those that repeat.
func uniquePackNames(packs []Pack, used map[string]bool) []Pack {
	names := map[string]bool{}
	kept := make([]Pack, 0, len(packs))
	for _, pack := range packs {
		if used[pack.ID] {
			name := trimJS(pack.Name)
			pack.Name = unique(names, name, numbered(name))
			kept = append(kept, pack)
		}
	}
	return kept
}

// uniqueListNames trims the names of the lists and numbers those that repeat in their pack, in place.
func uniqueListNames(lists []List) []List {
	listNames := map[string]map[string]bool{}
	for i, list := range lists {
		taken := listNames[list.PackID]
		if taken == nil {
			taken = map[string]bool{}
			listNames[list.PackID] = taken
		}
		name := trimJS(list.Name)
		lists[i].Name = unique(taken, name, numbered(name))
	}
	return lists
}
