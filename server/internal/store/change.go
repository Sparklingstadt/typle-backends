package store

import (
	"fmt"

	"github.com/sparklingstadt/typle-r/server/internal/workspace"
)

// A change, encoded as it is stored, before it is written.

// encodedChange is a Change as WriteRevision writes it. nil is a part left out: NULL in the queries of writeRevision.
// Lists, as the column keeps them, have no records: rows are their records, for typle_records.
type encodedChange struct {
	lists, packs, devices, interests []byte
	rows                             []recordRow
}

// encodeChange checks and encodes change for WriteRevision, without the database. Lists sent without packs are each
// put in a pack of their own, as Decode repairs lists without one, and change is given those packs.
func encodeChange(owner string, change *Change) (*encodedChange, error) {
	parts := &encodedChange{}
	// full is the lists with their records, as the app sent them.
	var full []byte
	var err error
	// Packs are saved only with their lists.
	if change.HasPacks && !change.HasLists {
		return nil, &workspace.Error{Key: "invalidWorkspace"}
	}
	if change.HasLists && !change.HasPacks {
		if err := packLists(owner, change); err != nil {
			return nil, err
		}
	}
	if change.HasLists {
		if full, err = parts.encodeLists(change); err != nil {
			return nil, err
		}
	}
	if change.HasDevices {
		if parts.devices, err = workspace.EncodeDevices(change.Devices); err != nil {
			return nil, err
		}
	}
	if change.HasInterests {
		// Interests the app would refuse are refused as the request would be.
		if err := workspace.CheckInterests(change.Interests); err != nil {
			return nil, &workspace.Error{Key: "invalidWorkspace"}
		}
		if parts.interests, err = workspace.EncodeInterests(change.Interests); err != nil {
			return nil, err
		}
	}
	// A part left out is saved as it is, so only the other one can be refused; it is read with an empty one.
	if _, err := workspace.Decode(orEmpty(full), orEmpty(parts.devices), orEmpty(parts.packs)); err != nil {
		return nil, fmt.Errorf("changed workspace of %s: %w", owner, err)
	}
	return parts, nil
}

// encodeLists encodes the lists and packs of change into parts, with the lists' records as rows, and returns the
// lists with their records (full), as the app sent them, for encodeChange to check.
func (parts *encodedChange) encodeLists(change *Change) (full []byte, err error) {
	if err := workspace.CheckLimits(change.Lists); err != nil {
		return nil, err
	}
	if parts.packs, err = workspace.EncodePacks(change.Packs); err != nil {
		return nil, err
	}
	if parts.rows, err = recordRows(change.Lists); err != nil {
		return nil, err
	}
	// Followed packs' lists are saved without their words, which the catalog has.
	slim := workspace.WithoutFollowedWords(change.Packs, change.Lists)
	if full, err = workspace.Encode(slim); err != nil {
		return nil, err
	}
	if parts.lists, err = workspace.Encode(withoutRecords(slim)); err != nil {
		return nil, err
	}
	return full, nil
}

// packLists puts each list of change, sent without packs, in a pack of its own, as Decode repairs lists without one.
func packLists(owner string, change *Change) error {
	if err := workspace.CheckLimits(change.Lists); err != nil {
		return err
	}
	encoded, err := workspace.Encode(change.Lists)
	if err != nil {
		return err
	}
	repaired, err := workspace.Decode(encoded, []byte("[]"), []byte("[]"))
	if err != nil {
		return fmt.Errorf("changed workspace of %s: %w", owner, err)
	}
	change.Lists, change.Packs, change.HasPacks = repaired.Lists, repaired.Packs, true
	return nil
}

func orEmpty(part []byte) []byte {
	if part == nil {
		return []byte("[]")
	}
	return part
}

// nullable is a part to write as a query argument: NULL when it is left out.
func nullable(part []byte) any {
	if part == nil {
		return nil
	}
	return string(part)
}
