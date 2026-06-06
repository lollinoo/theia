//go:build windows

package main

// This file defines icon windows behavior for the Winbox bridge command.

import _ "embed"

//go:embed icon.ico
var iconBytes []byte
