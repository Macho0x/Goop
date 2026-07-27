// Package version exposes the Goop compiler version string.
//
// The embedded VERSION file in this package must stay in sync with the
// repository-root VERSION file. TestSyncWithRootVERSION enforces that.
package version

import (
	_ "embed"
)

//go:embed VERSION
var Version string
