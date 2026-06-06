package version

// This file defines version version metadata exposed to the application.

// Set at build time via -ldflags.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)
