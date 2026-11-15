// Package workspace reads a Typle workspace: the lists, word packs and devices of an account, as the app saves them in
// the typle_workspaces table. The checks follow decodeLists (src/core/wordLists.ts), decodePacks
// (src/core/wordPacks.ts) and decodeDevices (src/core/devices.ts), so data the app would read is read here and data it
// would refuse is refused. The one difference is that null in an optional field reads as the field left out.
package workspace

import (
	"slices"
	"time"
)

const (
	maxDevices      = 100
	maxDeviceID     = 100
	maxDeviceName   = 100
	maxDeviceLayout = 200
	DefaultDeviceID = "default"
)

type Word struct {
	Display    string `json:"display"`
	Input      string `json:"input"`
	Annotation string `json:"annotation"`
	// A missing type is a legacy/plain word. Keep it missing when writing old data.
	InputType string `json:"input_type,omitempty"`
	// Extra holds the stored fields Typle does not know (see extra.go).
	Extra string `json:"-"`
}

// WritingLanguages are the languages a list can be practised in by handwriting (WRITING_LANGUAGES in
// src/core/wordLists.ts).
var WritingLanguages = []string{"en", "ja", "zh-Hans", "zh-Hant", "ko", "fr", "de", "es", "ru", "cs"}

// Writing is the mark of a list that can be practised by handwriting (Typle Writing). Only the language is kept.
type Writing struct {
	Language string `json:"language"`
}

func validWritingLanguage(language string) bool { return slices.Contains(WritingLanguages, language) }

type Device struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Layout string `json:"layout"`
	// Extra holds the stored fields Typle does not know, of a device copied into a record.
	Extra string `json:"-"`
}

// Record is a practice run as it is stored. Records saved by older versions may have no id, mistakes, accuracy,
// wordCount or device.
type Record struct {
	ID        *string  `json:"id,omitempty"`
	Time      float64  `json:"time"`
	Date      string   `json:"date"`
	Mistakes  *int     `json:"mistakes,omitempty"`
	Accuracy  *float64 `json:"accuracy,omitempty"`
	WordCount *int     `json:"wordCount,omitempty"`
	Device    *Device  `json:"device,omitempty"`
	// Mode is RecallMode or WritingMode for a run practised that way; "" for a normal run, which has no mode key.
	Mode string `json:"mode,omitempty"`
	// Missed is the words that went wrong in the run; none in runs saved before v2.42.0.
	Missed []MissedWord `json:"missed,omitempty"`
	// At is Date as a time, for ordering and filtering.
	At    time.Time `json:"-"`
	Extra string    `json:"-"`
}

type List struct {
	ID   string
	Name string
	// Levels pick the list within its pack on the practice screen (nil when it has none; an empty array as stored stays
	// empty). See levelPath.
	Levels []string
	// Writing is set when the list can be practised by handwriting; nil otherwise.
	Writing *Writing
	// PackID is the word pack the list belongs to. Every list Decode returns has one.
	PackID string
	// Group is the group of a list saved before packs existed, which Decode makes a pack of. Encode never writes it; only
	// EncodeWithLabels does, for lists sent by an app that does not know packs.
	Group string
	// Category and Tags are those of the list's pack, as the app's viewLists shows them: "" and nil when it has none.
	// They are not stored with the list; ApplyListChange puts a change to them back into the pack.
	Category  string
	Tags      []string
	Words     []Word
	Records   []Record
	CreatedAt string
	// Origin is set on the lists of a followed official pack: the official list it is kept equal to (see followOfficial).
	Origin *ListOrigin
	Extra  string
}

// ListOrigin is the origin of a list of a followed pack. Extra holds the fields of it Typle does not know.
type ListOrigin struct {
	ListID string `json:"listId"`
	Extra  string `json:"-"`
}

type Workspace struct {
	Lists   []List
	Packs   []Pack
	Devices []Device
}

// Find returns the list with this id, or nil.
func (w *Workspace) Find(id string) *List {
	if at := index(w, id); at >= 0 {
		return &w.Lists[at]
	}
	return nil
}

// FindPack returns the pack with this id, or nil.
func (w *Workspace) FindPack(id string) *Pack {
	if at := packIndex(w, id); at >= 0 {
		return &w.Packs[at]
	}
	return nil
}
