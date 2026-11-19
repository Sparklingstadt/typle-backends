package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"unicode/utf16"
)

// decodeWords reads a list's words: each has the text to show, to type and an annotation, and shows and types some.
func decodeWords(raw []*rawWord) ([]Word, error) {
	words := make([]Word, 0, len(raw))
	for index, word := range raw {
		if word == nil || word.Display == nil || word.Input == nil || word.Annotation == nil {
			return nil, fmt.Errorf("word %d: display, input and annotation are required", index+1)
		}
		if trimJS(*word.Display) == "" || trimJS(*word.Input) == "" {
			return nil, fmt.Errorf("word %d: display and input must not be empty", index+1)
		}
		words = append(words, Word{Display: *word.Display, Input: *word.Input, Annotation: *word.Annotation, InputType: word.InputType, Extra: word.extra})
	}
	return words, nil
}

// decodeStrings reads an optional array of strings: nil when it is left out, false when an item is not a string.
func decodeStrings(raw []*string) ([]string, bool) {
	if raw == nil {
		return nil, true
	}
	values := make([]string, 0, len(raw))
	for _, value := range raw {
		if value == nil {
			return nil, false
		}
		values = append(values, *value)
	}
	return values, true
}

// decodeRecords reads a list's records, naming the one that cannot be read.
func decodeRecords(raw []*rawRecord) ([]Record, error) {
	records := make([]Record, 0, len(raw))
	for index, item := range raw {
		record, err := decodeRecord(item)
		if err != nil {
			return nil, fmt.Errorf("record %d: %w", index+1, err)
		}
		records = append(records, record)
	}
	return records, nil
}

// writingOf follows isWritingMark (src/lib/wordLists.ts): the mark is an object whose only field is a listed language.
// Anything else makes the list unreadable, as a malformed list field does; null reads as left out.
func writingOf(raw json.RawMessage) (*Writing, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	var language string
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 1 || json.Unmarshal(fields["language"], &language) != nil ||
		!validWritingLanguage(language) {
		return nil, errors.New("writing must be an object with a supported language only")
	}
	return &Writing{Language: language}, nil
}

// originOf follows decodeLists (src/lib/wordLists.ts): a list's origin is kept when it is an object with a string
// listId, with the other fields it has; any other origin is dropped.
func originOf(raw json.RawMessage) (*ListOrigin, error) {
	var fields map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &fields) != nil {
		return nil, nil
	}
	var id string
	if json.Unmarshal(fields["listId"], &id) != nil || string(fields["listId"]) == "null" {
		return nil, nil
	}
	extra, err := unknown(raw, "listId")
	if err != nil {
		return nil, err
	}
	return &ListOrigin{ListID: id, Extra: extra}, nil
}

func decodeRecord(raw *rawRecord) (Record, error) {
	if raw == nil || raw.Time == nil || *raw.Time < 0 || raw.Date == nil {
		return Record{}, errors.New("time must be a number of 0 or more and date is required")
	}
	at, ok := ParseTime(*raw.Date)
	if !ok {
		return Record{}, fmt.Errorf("date %q is not a supported date", *raw.Date)
	}
	if raw.Accuracy != nil && (*raw.Accuracy < 0 || *raw.Accuracy > 100) {
		return Record{}, errors.New("accuracy must be between 0 and 100")
	}
	if raw.Mistakes != nil && *raw.Mistakes < 0 {
		return Record{}, errors.New("mistakes must be 0 or more")
	}
	if raw.WordCount != nil && *raw.WordCount < 1 {
		return Record{}, errors.New("wordCount must be 1 or more")
	}
	if raw.Device != nil && !isDevice(*raw.Device) {
		return Record{}, errors.New("device is invalid")
	}
	mode := ""
	if raw.Mode != nil {
		// An explicit null is refused as the app does: the value is "recall" or "writing", or the key is absent.
		var value string
		if json.Unmarshal(raw.Mode, &value) != nil || value == "" || !validMode(value) || string(bytes.TrimSpace(raw.Mode)) == "null" {
			return Record{}, errors.New(`mode must be "recall" or "writing" when present`)
		}
		mode = value
	}
	missed, ok := decodeMissed(raw.Missed)
	if !ok {
		return Record{}, errors.New("missed must be up to 20 words, each with a count of 1 or more")
	}
	return Record{ID: raw.ID, Time: *raw.Time, Date: *raw.Date, Mistakes: raw.Mistakes, Accuracy: raw.Accuracy,
		WordCount: raw.WordCount, Device: raw.Device, Mode: mode, Missed: missed, At: at, Extra: raw.extra}, nil
}

// JavaScript measures string length in UTF-16 code units.
func length(value string) int {
	return len(utf16.Encode([]rune(value)))
}

func isDevice(device Device) bool {
	return device.ID != "" && length(device.ID) <= maxDeviceID &&
		trimJS(device.Name) != "" && length(device.Name) <= maxDeviceName &&
		length(device.Layout) <= maxDeviceLayout
}

func decodeDevices(raw []json.RawMessage) ([]Device, error) {
	invalid := errors.New("devices are invalid")
	if len(raw) > maxDevices {
		return nil, invalid
	}
	devices := make([]Device, 0, len(raw))
	for _, item := range raw {
		var fields struct {
			ID     *string `json:"id"`
			Name   *string `json:"name"`
			Layout *string `json:"layout"`
		}
		if err := json.Unmarshal(item, &fields); err != nil || fields.ID == nil || fields.Name == nil || fields.Layout == nil {
			return nil, invalid
		}
		device := Device{ID: *fields.ID, Name: *fields.Name, Layout: *fields.Layout}
		// The built-in device is never stored with the registered ones.
		if !isDevice(device) || device.ID == DefaultDeviceID ||
			slices.ContainsFunc(devices, func(other Device) bool { return other.ID == device.ID }) {
			return nil, invalid
		}
		devices = append(devices, Device{ID: device.ID, Name: trimJS(device.Name), Layout: trimJS(device.Layout)})
	}
	return devices, nil
}
