# Authenticated historical task evidence

`zentao_my_task_history` is a read-only aggregate using the caller's existing token.
Inputs: `year` (0 defaults to Shanghai current year), `mode` (`year` default or `all`), and either `execution_ids` or explicit `scan_all_visible: true`. The two scope selectors are mutually exclusive. Missing scope is rejected before upstream reads. Execution IDs must belong to the authenticated `/user` profile's `view.sprints`.

`year` filters list-row date fields before loading details. It cannot prove full annual coverage: tasks with only historical actions in the year can be omitted. `complete` is therefore always false in this mode; `scan_complete` reports whether the selected scan itself finished cleanly. `all` checks details for all discovered tasks in the explicitly selected visible scope. Both modes have a 50-second query budget and a separate 3-second final identity/visibility check.

Participation uses exact account values in assignedTo, finishedBy, closedBy, action.actor, and assigned-action history entries whose field is exactly assignedTo. It never searches descriptions or matches account substrings or display names. Missing actions, unresolvable assignment targets, name-only identity evidence, failed pagination, task/execution mismatches, or changed visibility prevent complete evidence. Identity/visibility changes discard the result.

Current overdue means deadline strictly before the real Shanghai date and status wait/doing/pause. Done, closed, cancel, cancelled and unknown status never count as current overdue. Finished-late uses finishedDate, falling back to closedDate with explicit `closedDate_proxy` basis. Closure is not asserted to be actual completion. Counts only include tasks with proven authenticated-user relations. At most 200 matched items are returned, with truncation clearly marked; counts still describe scanned matches.

Completeness is scoped to currently visible executions and retained structured history. It does not establish coverage of inaccessible/deleted historical tasks. No writes, user-selectable identity, forged today, model generation, or group messaging are performed. Hermes' independent generation/review gates remain separate and unchanged.
