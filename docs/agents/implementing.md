# Implementing a spec

For whoever orchestrates `/implement-spec` and for the implementers it launches.

## Sequencing

Run tickets in parallel unless they edit the same pipeline files: tickets that touch `internal/bot/input.go` or `internal/bot/confirmation.go` run one at a time, each merged before the next starts. Merges in `bot.Deps`, `app.go` and the `bottest` harness are mechanical; let them run in parallel.

## Commits

An implementer commits at the end of each acceptance criterion, with a normal message. A stopped or killed agent then leaves a readable trail in its worktree.

## Spec deviations

An implementer reports a spec deviation it finds, in its own ticket or an earlier one, in its hand-off and does not open an issue. The orchestrator turns each reported deviation into a ticket before the next ticket that touches the same code starts.
