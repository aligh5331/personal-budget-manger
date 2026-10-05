# Jev as the only LLM call

Research for ticket #17 (map #1). Run 2026-10-05 against `POST https://api.metisai.ir/typesafe/v1/systemone` (model `jev-latest`, Metis key as Bearer). Script: `scripts/jev_sole_call.py` (throwaway, on branch `research/jev-sole-call`). Cases are cited by number from the gitignored `samples/forwarded-messages.txt`; no raw bank text is quoted here.

## Answer

On the 18 sample cases Jev **matches** `gpt-4.1-mini` on amount, Direction and date, and gives usable Category answers, at about **$0.00006 per Input** (versus about $0.0004 for `gpt-4.1-mini` extraction plus a separate Jev Category call). It is not a drop-in replacement: code must still do everything that is not a choice (below). Both options cost cents a month, so cost does not decide.

## Setup

- One request per Input, four Choice questions on the same `state` (the Input with digits normalised to ASCII): `amount`, `direction`, `category`, `date`.
- Code builds the options. Amount: every number-like token left after removing date and time tokens, plus `none`. Each option's `criteria` text is the printed line the number was found on (e.g. the "amount" line versus the "balance" line); this carries most of the accuracy. Date: every `YYYY/MM/DD` or year-less `MMDD-HH:MM` token plus `none`. Direction: out / in / ambiguous. Category: the 15 + 2 starting Categories (English key, English description, Persian hint) plus `uncategorized`.
- 18 cases, 2 runs, plus one run with the option order reversed. Persian text, Persian hints and English keys all worked; every call returned 200, no retries needed, response uses `probabilities` plus `confidence` as TypeSafe documents.

## Results (18 cases)

| Field | Correct | Notes |
|---|---|---|
| Amount | 17/18 | Case 16 (two messages): picks one of the two amounts (a single Choice cannot return both). Case 13 (no amount line): correctly returns `none`. Cases 14 and 15 (toman note) correct. Never picked the balance. |
| Direction | 17/17 scorable | Case 8 (equal +/- pair) returns `ambiguous` at 0.30 confidence, as expected: code must merge the pair. Case 13 returns `out` at 0.65. Case 18 (hashtag says card, sign is minus): `out`. |
| Category | 17/17 scorable | Accepting the alternatives the sample file allows (4 education, 10 fuel, 12 entertainment, 14 groceries, 17 food). Case 5 (no note) returns `uncategorized` at 0.52; case 18 (loan given, no matching Category) returns `other` at 0.55. Case 8 (internal) gets `uncategorized`, irrelevant. |
| Date | 18/18 | Year-less `MMDD` and full Jalali both picked right; case 14 (no date) returns `none`. Case 16 picks the first message's date (the reversed-order run picked the second). |

Stability: all 18 cases returned identical answers in both runs. With the Amount, Category and Date options in reverse order the answers were the same on all 18 (only case 16's date switched between its two messages), so no option-order bias showed up here.

Confidence: 1.0 on most Amount, Direction and Date answers. Lowest: Category 0.52 (case 5), 0.55 (case 18), 0.75 (case 10), Direction 0.30 (case 8), 0.65 (case 13), Amount 0.72 and Date 0.44 (case 16). The low values line up with the truly unclear cases, so gating at 0.7 as in the contract would have flagged or asked on cases 5, 8, 13, 16, 18 and nothing else, plus case 10's Category at 0.75 just passing.

Compared with `gpt-4.1-mini` (`extraction-llm.md`, prompt v1, 3 repeats): 51/54 amount, 51/54 direction, 51/54 date, 48/54 description; the misses were dropped calls, not wrong answers. Jev had no dropped calls in 54 requests (36 + 18). Jev's Category step also did what the separate Jev call would do, so the comparison for Category is simply Jev against nothing yet.

## Cost and latency

- About 1,350 input tokens per Input (range 1,150 to 1,540; the options and Persian hints dominate, Persian tokenises heavily) and about 310 output tokens. Metis lists Jev at $0.046 per 1M input and $0 output: **about $0.000062 per Input**, about $0.01 a month at 5 Inputs a day, $0.04 at 20.
- `gpt-4.1-mini` extraction is about $0.00037 per call, so Jev-only is roughly 6x cheaper per Input, but the saving is about $0.06 a month.
- Latency p50 2.6 s, p95 6.4 s over 36 calls (home connection), similar to `gpt-4.1-mini` (2.6 s / 4.9 s).
- Splitting a multi-message Input into one request per bank message (needed, see below) multiplies tokens and calls by the message count (up to 5).

## What Jev cannot do (code must cover)

- **Units.** Jev returns which token, not its unit. Code knows which line a token came from, so bank lines are rial and a note-only number is toman (as in #10). Multipliers and words are code's job: an extra probe with a note like "450 هزار تومن" yielded the token 450 (so ×1000 was missing), and "دو میلیون و دویست هزار تومن" yielded no number token at all (`none`). The LLM handles both.
- **Per-message split.** One Choice picks one amount, so case 16 (two messages, one note) needs code to split the Input into messages first. That needs a bank-message boundary rule, which is bank-specific, against the convention that other banks need no new rules.
- **Internal-pair merge.** Code, same as with the LLM.
- **Description / note.** Jev returns no text. Code must cut the Owner's note out of the Input, which also needs the bank-message boundary (the note comes before, after or between). The LLM copies it as a top-level field.
- **Is it a transaction?** Not part of the four questions. An extra Choice question ("already happened versus ad, reminder, future bill") rejected an ad and a future-tense instalment notice (both `no` at 1.0), but wrongly said `no` on a note-only Input ("lunch 350 toman", confidence 0.40) and on a two-item note (0.85), and the Amount/Category answers for the ad (Category `bills` at 0.91) were confidently wrong, so the code must apply this gate first, and only to bank-style Inputs.
- **Free-form dates** («دیروز», «امروز») are not tokens; code must map them.
- **Several amounts in one note** (e.g. "cinema 450 هزار, popcorn 80 هزار") gives one pick at 0.57 confidence.

## Caveats

- 18 cases, one bank (Melli) in two formats plus two note-only cases, all drafted by the Owner and the agent; Jev's option text in my script was tuned by one pass only, not iterated, but the 17 + 17 + 18 scoring is on the same cases it ran on, so treat it as optimistic like v2 in `extraction-llm.md`.
- The criteria text (the printed line next to each number) is what separates the amount from the balance; without it Jev would have to guess among bare numbers. That makes code build per-line option descriptions, a design cost, not a Jev capability.
- Cost uses documented Metis prices times `usage` token counts; billing unit and USD-to-toman on Metis remain unverified.
- Latency measured from a home connection, not the lab.
