# How broad an Input Jev can handle

Research for ticket #21 (map #1). Run 2026-10-06 against `POST https://api.metisai.ir/typesafe/v1/systemone` (model `jev-latest`). Script: `scripts/jev_breadth.py` (throwaway, extends `scripts/jev_sole_call.py`), results in `scripts/jev_breadth_results.json`. 43 cases, 2 runs, all 86 calls finished (after retries). Cases are cited by id (a1..h5). Cases marked REAL are built from the gitignored `samples/` and not quoted here; DRAFT cases were written by the agent (the notes are short Persian phrases, other-bank formats are **invented** from public descriptions, not real Mellat/Saman/Sepah messages, so treat the "other banks" row as format-robustness, not proof for those banks).

## Answer

Jev is reliable as a **chooser** over options that code builds: amount, Direction, date, Category and "relative date" all worked across note-only, other-format, multi-message, noisy and messy Inputs, at about 1.1 to 1.6 s and 1,630 input tokens per call (about $0.000075). It fails wherever the answer is not a token the code already extracted (number words, free-form dates beyond a fixed list, text spans) and where one question needs several answers (multi-message, multi-item). Its "is this a transaction" answer is **not usable on note-only Inputs**. Every verdict below assumes code does the extraction, splitting and gating around it.

## Setup differences from ticket #17

- Amount options: number tokens, plus **multiplier merge** by code («۴۵۰ هزار» becomes one option 450000, «۱.۵ میلیون» 1500000). Decimal and Arabic-Indic digits normalised.
- Date options: explicit tokens plus fixed `today`, `yesterday`, `day_before`, `written` (month name, weekday, "last week") and `none`.
- Two new questions: `tx` (money actually moved vs ad, reminder, balance-only, OTP, chat) and `count` (zero / one / two / three_plus).
- Retries on 5xx (see Availability).

## Results by dimension

Accuracy counts both runs, only fields the case defined an expectation for.

| Dimension | Amount | Direction | Category | Date | tx gate | Verdict |
|---|---|---|---|---|---|---|
| Note-only (a1-a5) | 10/10 | 8/8 | 8/8 | n/a | **7/10** | works for the four fields; the gate fails (a1, a3 answered `no` at 0.13 and 0.37-0.46) |
| Multiplier in number (b1, b4) | 4/4 | 4/4 | 4/4 | n/a | 2/4 (conf 0.26-0.46) | works with code help (merge) |
| Number words (b2, b3, b5) | **0/6** (`none`) | b2 `ambiguous` 0.17 | ok | n/a | b2, b3 `no` | **fails**, LLM or a Persian number-word parser must do it; Jev answers `none` at 1.0 for b2, b5, so the failure is silent |
| Several items in a note (c1-c4) | picks one (c1 450000 at 0.62, c3 200000 at 0.47, c2 500000 at 0.92); c4 (one total, one sub-amount) correct | ok | ok | n/a | ok | `count` right 8/8 and low confidence flags the ambiguity, but only one amount is returned: **works with code help** (split text first) or fails |
| Relative or missing dates (d1-d6) | 12/12 | 12/12 | 12/12 | 10/12 | 12/12 | works: «دیروز», «پریروز», explicit note date, "written" all right; no date returns `none` at 0.99 (acceptable, code defaults to today) |
| Beyond Melli (e1-e8; 3 REAL Melli/Bale formats, 5 DRAFT) | 16/16 | 14/16 | 10/10 | 12/12 (e5 year-less `07/10` came back `written`) | 16/16 | works; e8 (English `Cr ... IRR`) Direction `ambiguous` 0.25 then `out` 0.24 (wrong, low confidence); a Category on a plain Bale transfer (e2) returned `uncategorized`, fine |
| 2 to 5 bank messages (f1-f3) | picks one (0.28 to 0.97) | ok | ok | picks one | yes | `count` correct 6/6 (two, three_plus); amount confidence 0.28-0.7 on 3+ messages. Needs code split; Jev can tell *that* there are several |
| Noise (g1-g7) | g1 picks 200000 at 0.47 | n/a | confidently wrong (g2 `bills`, g6 `loan_installment`) | n/a | **14/14 `no`**, all at 0.99-1.0 | the gate works on bank-style and ad-like noise; `count` alone says `one` for the three that look like real notices (g2, g6, g7) so use `tx`, not `count`. g7 (future promise «فردا باید...») `no` at 1.0 |
| Long, mixed, typos, digits (h1-h5) | 10/10 | 8/10 | 6/8 | 2/2 | 10/10 | works for long note, Latin words, Arabic-Indic digits and ASCII digits (h1, h3-h5). Typo-heavy note h2 («نهر ۳۵۰ تمن»): Direction `ambiguous` 0.3 and Category `uncategorized` 0.5, both low confidence, the right outcome is a Follow-up or Uncategorized |

Stability: 37 of 43 cases gave identical answers in both runs. The six that moved (a1, b3, c1, e8, g5, g7) are all low-confidence answers or noise.

## Confidence as a gate

Low confidence marks the problems in this data. Direction below 0.7: b2, e8, h2 (all genuinely unclear or wrong). Amount below 0.7: c1, c3, f1, f3, b3, g1. Category 0.5 to 0.55: h2, e8. `tx` below 0.5 on valid notes: a1, a3, b1, b3, b4, d6 (so it cannot gate notes). Answers wrong **at high confidence** were: number-word amount `none` at 1.0 (b2, b5), noise Category (g2, g6, g7) and `count` on notices. A confidence gate catches nothing in those, they need code gates.

## What code must own (carried from #17, now measured)

1. The is-it-a-transaction gate: apply `tx` only to bank-style messages (they have a label, balance, or a bank name), not to note-only Inputs.
2. Number words and any amount that is not a digit token (b2, b3, b5). A tiny Persian number-word parser, or ask the Owner to type digits, or an LLM. This is a silent failure: Jev returns `none`.
3. Splitting multi-message and multi-item Inputs. Jev's `count` can detect the need, code must do the split and one call per piece (up to 5 calls, about $0.0004).
4. Merging «هزار/میلیون» multipliers, decimal handling, `۳ تا ۱۵ هزارتومنی` style products (b3 gives 15000 at 0.15).
5. Date defaults: `none` and `today` are equivalent; map `yesterday`, `day_before`, `written` to dates, and ask when `written` (d4, d5, e5) since the month name or weekday is not resolved by Jev.

## Availability (not in the ticket, but it decided whether the run worked)

Around the start of the run Metis returned **503 "no healthy upstream" / `model_unavailable` on 88 of 90 calls** for about a minute, then recovered (curl check: 503, 200, 200). A later run failed on the home connection with DNS errors (not Metis). With 4-second-backoff retries every other call succeeded on the first or second try. The bot needs retries and a user-visible "try again" path for Jev calls; a Jev-only pipeline has no fallback model when this happens.

## Caveats

- 43 cases, one real bank (Melli, 3 formats), five DRAFT other-bank formats. Real Mellat/Saman/Sepah messages may differ.
- Notes and noise are drafted; scoring is per field against my own expectations and I tuned option wording once, so treat results as optimistic.
- Billing unit and USD-to-toman on Metis remain unverified.
