// Package instructions embeds CHANGE_REVIEW.md, the agent context file for
// this binary's schema version.
package instructions

import _ "embed"

// Text is the content of CHANGE_REVIEW.md, printed by `change-view
// instructions` so an agent learns how to write a review file this binary
// accepts.
//
//go:embed CHANGE_REVIEW.md
var Text string
