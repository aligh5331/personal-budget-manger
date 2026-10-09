# Coding standards

Read at review time. Mechanical rules are lint or tests, not prose: see `scripts/check.sh`.

## Judgement calls

- **Reuse before writing.** Search `internal/bot` for an existing parser, pager, view or helper before adding a second one. Parallel tickets duplicated callback parsing, paging and Direction mapping once; the fix cost a whole review pass.
- **A typed value beats a string with switches.** When the same string vocabulary (Direction, flag reason, callback action) is switched on in more than one file, give it a type and constants in `internal/storage`.
