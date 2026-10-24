---
name: task-review
description: Use when you finish a coding task, or when asked to describe an existing commit for review. Writes and validates a task-view review file (.review/change-<task>.json) that maps the change - the pieces touched, why, how they relate and the flows they form - so the reviewer reads it as one page instead of rebuilding that structure from a flat diff.
---

# Task review

When a task is complete, hand it over as a review file instead of only a
summary in chat. `task-view` turns it into one page that shows how the change
fits together: a graph of the pieces you touched, its flows step by step, and
each piece's code next to your reason for it. It checks every reference you
give against Git so the map matches the real diff.

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
without changing any code: `task-view gather --base <sha>^ --tip <sha>`
gives its evidence.
