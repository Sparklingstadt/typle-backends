package workspace

import (
	"encoding/json"
	"errors"
)

// Each stored shape keeps the fields it does not know.

func (r *rawWord) UnmarshalJSON(data []byte) (err error) {
	type plain rawWord
	if err = json.Unmarshal(data, (*plain)(r)); err == nil {
		if r.InputType, err = wordInputType(data); err == nil {
			r.extra, err = unknown(data, "display", "input", "annotation", "input_type")
		}
	}
	return err
}

// The JSON field is optional, but a supplied value must be one of the three exact strings.
// In particular, null and an empty string do not mean the same thing as a missing field.
func readInputType(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || value == "" || !validInputType(value) {
		return "", errors.New("input_type must be command, code or word")
	}
	return value, nil
}

func wordInputType(data []byte) (string, error) {
	var fields struct {
		InputType json.RawMessage `json:"input_type"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return "", err
	}
	return readInputType(fields.InputType)
}

func validInputType(value string) bool {
	return value == "" || value == "command" || value == "code" || value == "word"
}

// Word is also read directly by list/pack mutation requests, outside rawList decoding.
func (w *Word) UnmarshalJSON(data []byte) (err error) {
	type plain Word
	if err = json.Unmarshal(data, (*plain)(w)); err == nil {
		w.InputType, err = wordInputType(data)
	}
	return err
}

func (r *rawRecord) UnmarshalJSON(data []byte) (err error) {
	type plain rawRecord
	if err = json.Unmarshal(data, (*plain)(r)); err == nil {
		r.extra, err = unknown(data, "id", "time", "date", "mistakes", "accuracy", "wordCount", "device", "mode", "missed")
	}
	return err
}

func (r *rawList) UnmarshalJSON(data []byte) (err error) {
	type plain rawList
	if err = json.Unmarshal(data, (*plain)(r)); err == nil {
		r.extra, err = unknown(data, "id", "name", "category", "tags", "group", "packId", "levels", "writing", "createdAt", "words", "records",
			"origin", "shared", "shareId")
	}
	return err
}

func (d *Device) UnmarshalJSON(data []byte) (err error) {
	type plain Device
	if err = json.Unmarshal(data, (*plain)(d)); err == nil {
		d.Extra, err = unknown(data, "id", "name", "layout")
	}
	return err
}

// Words, records, list origins and devices are written with their unknown fields, both to the database and in the
// API's answers.

func (w Word) MarshalJSON() ([]byte, error) {
	type plain Word
	known, err := marshal(plain(w))
	return withExtra(known, w.Extra), err
}

func (r Record) MarshalJSON() ([]byte, error) {
	type plain Record
	known, err := marshal(plain(r))
	return withExtra(known, r.Extra), err
}

func (o ListOrigin) MarshalJSON() ([]byte, error) {
	type plain ListOrigin
	known, err := marshal(plain(o))
	return withExtra(known, o.Extra), err
}

func (d Device) MarshalJSON() ([]byte, error) {
	type plain Device
	known, err := marshal(plain(d))
	return withExtra(known, d.Extra), err
}
