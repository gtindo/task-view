You have just finished a coding task in this repository. Here is what you did:

{{brief}}

Now hand the work over for review. Run `task-view instructions` and follow
what it prints exactly: write the review file `{{review_file}}` for this task.
You started the task from commit `{{base}}`. {{state}}

Rules for this hand-over:

- In this environment `task-view` is `{{task_view}}`. Always call it by
  that absolute path, never as a bare `task-view`.
- Create or edit only `{{review_file}}`. Change no other file, and do not
  stage, commit, stash or check out anything.
- Read files and run read-only Git commands as you need. Do not run project
  tooling: no package managers, builds, tests or linters. Record tests as not
  run.
- Do not run `task-view open`.
- Stop when `validate {{review_file}} --format json` exits 0 with no
  `unmentioned_hunk` warning. Then reply with one line: `DONE`.
