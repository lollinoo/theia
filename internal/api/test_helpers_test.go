package api

import "errors"

// errMock is a sentinel error used by mock repositories in tests.
var errMock = errors.New("mock error")
