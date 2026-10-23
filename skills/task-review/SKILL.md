---
name: task-review
description: Use when you finish a coding task, or when asked to describe an existing commit for review. Writes and validates a task-view review file (.review/change-<task>.json) so a human can inspect the work as a checked graph, flows, diff map and claims.
---

# Task review

When a task is complete, hand it over as a review file that `task-view` can
check against Git, instead of only a summary in chat.

1. Run `task-view instructions` and follow the text it prints exactly. It is
   the format and the procedure for the version of `task-view` installed on
   this machine, so do not rely on a remembered copy.
   If `task-view` is not on `PATH`, follow `TASK_REVIEW.md` in the repo
   instead, and tell the user that `task-view` needs to be installed to view
   the review.
2. Write the file once the task is done, never during the work. If you change
   more for the same task, rewrite the whole file.
3. Do not stop until `task-view validate .review/change-<task>.json --format json`
   exits 0 with no `unmentioned_hunk` warnings, or the remaining ones are
   explained in `notes`.
4. End your reply with the command the user runs to review:

   ```bash
   task-view open .review/change-<task>.json
   ```

When asked to describe an existing commit, do the same for that commit
without changing any code.
