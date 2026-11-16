package web

import (
	"encoding/json"

	"github.com/sparklingstadt/typle-r/server/internal/store"
	"github.com/sparklingstadt/typle-r/server/internal/workspace"
)

// saved is the answer of GET and PUT. The lists, packs, devices and interests are the stored jsonb text, with the
// fields Typle does not know: Go decodes them only to check them (store.ReadRevision), so where Go and the app's
// decoders differ, the browser is the one to refuse the data (savedDataCorrupted). Packs are null for lists saved before
// packs existed, and for an account that has saved nothing. Interests are workspace.DefaultInterests (every option) when
// none have been saved, as the app reads them.
type saved struct {
	Lists     json.RawMessage `json:"lists"`
	Packs     json.RawMessage `json:"packs"`
	Devices   json.RawMessage `json:"devices"`
	Interests json.RawMessage `json:"interests"`
	Revision  int             `json:"revision"`
	AccountID string          `json:"accountId"`
}

// answer is the saved workspace as GET and PUT answer it.
func answer(result *store.Saved, account string) saved {
	interests := result.Interests
	if len(interests) == 0 {
		interests, _ = json.Marshal(workspace.DefaultInterests)
	}
	return saved{result.Lists, result.Packs, result.Devices, interests, result.Revision, account}
}
