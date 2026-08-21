# Remote Mission Status Contract

The Mobile Remote may display that a read-only Mission is currently active by observing the already-authenticated `/remote/api/status` fields `running` and `run_phase`.

It must not gain a separate Mission API, Mission start action, Mission/task identifiers, scheduler/task details, budget/accounting details, new tool authority, or new mutation authority from this UI surface.

The richer Mission scheduler/task/budget telemetry remains Desktop-only. Existing Remote stop behavior is unchanged; this slice adds no new control action.
