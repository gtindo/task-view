// Package instructions embeds CHANGE_REVIEW.md, the agent context file for
// this binary's schema version.
package instructions

import _ "embed"

//go:embed CHANGE_REVIEW.md
var Text string
