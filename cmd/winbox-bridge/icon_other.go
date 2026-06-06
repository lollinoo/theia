//go:build !windows

package main

// This file defines icon other behavior for the Winbox bridge command.

import _ "embed"

//go:embed icon.png
var iconBytes []byte
