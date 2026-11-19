package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
)

// The stored shapes, with pointers where the app requires a field to be present.
type rawWord struct {
	Display    *string `json:"display"`
	Input      *string `json:"input"`
	Annotation *string `json:"annotation"`
	InputType  string  `json:"input_type"`
	extra      string
}

type rawRecord struct {
	ID        *string         `json:"id"`
	Time      *float64        `json:"time"`
	Date      *string         `json:"date"`
	Mistakes  *int            `json:"mistakes"`
	Accuracy  *float64        `json:"accuracy"`
	WordCount *int            `json:"wordCount"`
	Device    *Device         `json:"device"`
	Mode      json.RawMessage `json:"mode"`
	Missed    json.RawMessage `json:"missed"`
	extra     string
}

// shared and shareId, which meant link sharing, are dropped. Other fields Typle does not know are kept.
type rawList struct {
	ID       *string   `json:"id"`
	Name     *string   `json:"name"`
	Category *string   `json:"category"`
	Tags     []*string `json:"tags"`
	Group    *string   `json:"group"`
	PackID   *string   `json:"packId"`
	Levels   []*string `json:"levels"`
	// Writing is read by writingOf, which refuses any shape but {"language": <listed>}; null reads as left out.
	Writing   json.RawMessage `json:"writing"`
	CreatedAt *string         `json:"createdAt"`
	Words     []*rawWord      `json:"words"`
	Records   []*rawRecord    `json:"records"`
	// Origin is read by originOf, which drops an origin of another shape.
	Origin json.RawMessage `json:"origin"`
	extra  string
}

// Decode reads a saved row: its lists, devices and packs columns. packs is nil (NULL) for lists saved before packs
// existed, which are turned into packs as the app's normalizeLibrary does; saved packs are repaired as it does. The
// library then follows the official catalog (followOfficial), as the app makes it do whenever it reads saved data.
func Decode(lists, devices, packs []byte) (*Workspace, error) {
	official, err := catalog()
	if err != nil {
		return nil, err
	}
	return decode(lists, devices, packs, official)
}

// decode is Decode following the official packs given.
func decode(lists, devices, packs []byte, official []OfficialPack) (*Workspace, error) {
	w, err := normalized(lists, devices, packs)
	if err != nil {
		return nil, err
	}
	w.Packs, w.Lists = followOfficial(w.Packs, w.Lists, official)
	viewLists(w)
	return w, nil
}

// normalized reads a saved row as the app's normalizeLibrary does, before it follows the catalog. Its lists do not carry
// their packs' labels yet (see viewLists).
func normalized(lists, devices, packs []byte) (*Workspace, error) {
	decoded, err := DecodeLists(lists)
	if err != nil {
		return nil, err
	}
	var rawDevices []json.RawMessage
	if err := json.Unmarshal(devices, &rawDevices); err != nil {
		return nil, fmt.Errorf("devices are not valid: %w", err)
	}
	registered, err := decodeDevices(rawDevices)
	if err != nil {
		return nil, err
	}
	var saved []Pack
	if packs != nil {
		if saved, err = decodePacks(packs); err != nil {
			return nil, err
		}
	}
	// Data without packs is no longer converted by the lists' groups (data from before v1.0.0 is deleted, see
	// db/migrations/0032_drop_legacy): a list without a pack is given one of its own, as any is.
	w := &Workspace{Devices: registered}
	w.Packs, w.Lists = repairLibrary(saved, decoded)
	return w, nil
}

// DecodeLists reads lists as they were saved, with the checks Decode makes of them, but without their packs: each keeps
// its packId, and any group, category and tags, as they are.
func DecodeLists(data []byte) ([]List, error) {
	var raw []*rawList
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("lists are not valid: %w", err)
	}
	if raw == nil {
		return nil, errors.New("lists must be an array")
	}
	return decodeLists(raw)
}

func decodeLists(raw []*rawList) ([]List, error) {
	lists := make([]List, 0, len(raw))
	ids := map[string]bool{}
	for index, item := range raw {
		list, err := decodeList(item)
		if err != nil {
			return nil, fmt.Errorf("list %d: %w", index+1, err)
		}
		if ids[list.ID] {
			return nil, fmt.Errorf("list %d: id %q is used twice", index+1, list.ID)
		}
		ids[list.ID] = true
		lists = append(lists, list)
	}
	return lists, nil
}

func decodeList(raw *rawList) (List, error) {
	if raw == nil || raw.ID == nil || *raw.ID == "" || raw.Name == nil || raw.CreatedAt == nil || raw.Words == nil || raw.Records == nil {
		return List{}, errors.New("id, name, createdAt, words and records are required")
	}
	// validateList: a name, at least one word, and no word without text to show or type.
	if trimJS(*raw.Name) == "" {
		return List{}, errors.New("name is empty")
	}
	// A list of a followed pack (with an origin) is saved without its words, which followOfficial takes from the catalog.
	if len(raw.Words) == 0 && raw.Origin == nil {
		return List{}, errors.New("words are empty")
	}
	words, err := decodeWords(raw.Words)
	if err != nil {
		return List{}, err
	}
	// The category and tags are optional; decodeLists checks only that they are a string and strings.
	category := ""
	if raw.Category != nil {
		category = *raw.Category
	}
	tags, ok := decodeStrings(raw.Tags)
	if !ok {
		return List{}, errors.New("tags must be strings")
	}
	// No tags are nil, as before; levels sent empty stay an empty list, as they are kept as they are.
	if len(tags) == 0 {
		tags = nil
	}
	levels, ok := decodeStrings(raw.Levels)
	if !ok {
		return List{}, errors.New("levels must be strings")
	}
	writing, err := writingOf(raw.Writing)
	if err != nil {
		return List{}, err
	}
	records, err := decodeRecords(raw.Records)
	if err != nil {
		return List{}, err
	}
	var group, packID string
	if raw.Group != nil {
		group = *raw.Group
	}
	if raw.PackID != nil {
		packID = *raw.PackID
	}
	origin, err := originOf(raw.Origin)
	if err != nil {
		return List{}, err
	}
	return List{ID: *raw.ID, Name: *raw.Name, Levels: levels, Writing: writing, PackID: packID, Group: group, Category: category, Tags: tags, Words: words, Records: records,
		CreatedAt: *raw.CreatedAt, Origin: origin, Extra: raw.extra}, nil
}
