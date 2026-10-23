// Package instructions embeds TASK_REVIEW.md, the agent context file for
// this binary's schema version.
package instructions

import _ "embed"

// Text is the content of TASK_REVIEW.md, printed by `task-view
// instructions` so an agent learns how to write a review file this binary
// accepts.
//
//go:embed TASK_REVIEW.md
var Text string
