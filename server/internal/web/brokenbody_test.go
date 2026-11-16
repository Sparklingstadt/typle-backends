package web

import "errors"

// A body that fails while it is read is a failed save, as in the route.
type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("connection reset") }
