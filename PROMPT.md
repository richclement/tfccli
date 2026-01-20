```markdown
Read `@specs/PRD.md` and `@specs/TASKS.md`.

## Hard constraints
- **ONLY work on ONE task per iteration.** Do not start a second task, even if you finish early.
- `@specs/TASKS.md` is the **source of truth** for task status and progress notes.
- Do not “fix” failures by disabling tests, weakening lint rules, or commenting-out assertions unless the PRD explicitly requires it. Prefer root-cause fixes.
- Make **one logical git commit** per completed task (or per iteration if the task isn’t finished), with a clear message.

## 0) Repo health (feedback loops)
Run these in order and fix anything that fails **until all are green**, even if unrelated to your task:
- `make fmt`
- `make lint`
- `make build`
- `make test`

Record in `@specs/TASKS.md`: commands run, failures encountered, what changed, final status.

## 1) Pick exactly one task
From `@specs/TASKS.md`, choose the **single highest priority incomplete** task, prioritizing:
- PRD/user impact
- unblockers
- risk/correctness/security
- tasks that are not blocked

## 2) Define “done” (before coding)
In `@specs/TASKS.md` under the chosen task, add a brief plan:
- acceptance criteria tied to PRD language
- verification approach (tests/commands)
- 3–7 bullet implementation plan

If the task is too large, **split it in `TASKS.md`** and complete **only the first subtask** this iteration.

If the task is **blocked** (missing requirements, dependency, unclear spec):
- document the blocker in `TASKS.md`
- add a TODO item that captures the question/requirement needed
- stop (do not guess or thrash)

## 3) Implement
Make the smallest coherent change set that satisfies the acceptance criteria.
Add/update tests when appropriate. Avoid drive-by refactors unless necessary.

## 4) Re-run feedback loops
Run `make fmt`, `make lint`, `make build`, `make test` again and fix failures until green.

## 5) Update `@specs/TASKS.md`
Append a timestamped progress note under the chosen task:
- what changed (files + summary)
- commands run + results
- what remains or why it’s done
- any follow-up tasks created
Update status (e.g., TODO → IN PROGRESS → DONE).

## 6) Self code review
Review your diff:
- fully implements the task with no workaround hacks
- handles edge cases / errors / security concerns
- tests are meaningful

## Exit condition
If (and only if) **all tasks in `TASKS.md` are DONE** and the feedback loops are green, output exactly:
**All tasks have been completed and the build and tests run correctly**
```
