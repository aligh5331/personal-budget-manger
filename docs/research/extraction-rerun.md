# Re-run of the 18 sample cases with the final extraction prompt

Ticket #27. Run 2026-10-06 from the lab (`gh-server`) against the live Metis API. Script: `scripts/extraction_rerun.py`. Cases are cited by number from the gitignored `samples/forwarded-messages.txt`; no raw bank text is quoted here.

## Setup

- Extraction: `gpt-4.1-mini`, `temperature: 0`, strict `json_schema`, the schema from `extraction-llm.md` (top-level `is_transaction` and `note`; per entry `amount` as printed, `amount_unit`, `direction` out/in/ambiguous, `date`, `time`, `confidence`). Today injected as 1405/07/13, the day the samples were written.
- Code side, as in #10 and #12: rial divided by 10; an equal out/in pair at the same date and minute merged into one `internal` Transaction; the note copied to every Transaction.
- Category: one Jev call per non-internal Transaction (`jev-latest` on `/typesafe/v1/systemone`), the full Input as `state`, options = the #14 Categories of the Direction's kind plus Uncategorized, confidence below 0.7 saves Uncategorized (#13).
- 18 cases x 3 runs = 54 extraction calls, 57 Jev calls. Retry-once was in place and never needed.

## Results

| Field | Correct | Misses |
|---|---|---|
| Amount | 51/54 | case 14, all 3 runs |
| Direction (incl. `internal` merge) | 54/54 | none |
| Date | 54/54 | none (year-less SMS dates and the note-only "today" included) |
| Category (acceptable set) | 54/54 | none |
| Note copied | 38/48 note-bearing calls | case 2 (3/3), 13 (3/3), 8 (2/3), 7 (1/3), 9 (1/3) |
| `is_transaction` | 54/54 true | none |

Per case: 1, 3, 4, 5, 6, 10, 11, 12, 15, 16, 17, 18 pass every field on every run. 7 and 9 pass except one dropped note each. 2, 8 and 13 pass except the note. 14 fails the amount.

Results were the same on all 3 runs apart from those single dropped notes. Latency from the lab: extraction p50 1.4 s, p95 2.1 s; Jev p50 0.5 s, p95 0.8 s. Whole run cost about $0.02 (about $0.0004 per Input).

## What checks out

- The untested parts of the #12 prompt now work: `time` per entry lets code merge case 8 into one `internal` Transaction (3/3), and the top-level `note` copied by code gives both case 16 Transactions the gift Category (3/3).
- Case 13 (no amount line) returns `amount: null` 3/3, never the balance. Case 18 (hashtag contradicts sign) is `out` 3/3.
- Jev confidence: case 5 (no note) lands around the 0.7 line (0.68 to 0.74) but Jev's own pick is Uncategorized anyway. Case 18 (lent to a friend) picks Other at 0.68, so it saves Uncategorized; there is no "money lent" Category, so either is acceptable. Fuel (case 10, 0.76 to 0.81) and Bills (case 15, 0.77 to 0.83) are the closest correct picks to the threshold.

## Cases that would change a contract decision

1. **Case 14: colloquial "تومن" on a note-only amount.** The note says 250 «تومن» for bread; the expected value is 250,000 toman, as people say it. The model returns 250 with unit toman, as the prompt asks ("as printed"), so code saves 250 toman. #10 and #12 put unit handling in code with the model reporting the printed number, so nothing in the contract catches this. Needs a rule: for example, a note-only amount below 1,000 toman is read as thousands, or the bot asks a Follow-up when a note-only amount is implausibly small.
2. **The Owner note is dropped on 10 of 48 calls.** The misses are consistent on three cases: a one-line note right after a bank message whose text looks like part of the message (case 2, a typo for loan installment; case 13, a pharmacy note after a message with no amount line) and the two-message transfer (case 8). Category is not affected, because Jev reads the full Input, and `raw_text` keeps everything. But `description` would be empty, and the description is what the Owner sees in `/transactions`. Options: a stronger prompt line (the note is any line not part of a bank message's template), code cutting the note out (code already owns the note cut-out per #17), or accepting it and showing `raw_text` instead.

## Caveats

- Same 18 cases the prompt was shaped on, one bank (Melli) in two formats plus one note-only case.
- The prompt wording is mine, built from the #12 shape; the build will write its own, so re-run these cases against the real prompt in the Go tests.
- Category scoring uses acceptable sets, not one right answer: case 4 Education or Entertainment, case 5 anything plausible, case 14 Groceries or Food, case 17 Snacks or Food, case 18 Other or Uncategorized.
