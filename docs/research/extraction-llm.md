# Extraction LLM: model, prompt shape, fallback

Research for ticket #12. Run 2026-10-05 against the live Metis API (`https://api.metisai.ir/openai/v1`, Bearer key). Sources: our own measurements (scripts were scratch, not committed) plus the Metis pricing JSON from `metis-api.md`. Cases are cited by number from the gitignored `samples/forwarded-messages.txt` (18 cases with expected results); no raw bank text is quoted here.

## Recommendation

- **Model: `gpt-4.1-mini`**, via `POST /openai/v1/chat/completions`, `temperature: 0`, `response_format` of type `json_schema` (strict) or plain JSON. About $0.00037 per call, so about $0.05 a month at 5 Inputs a day.
- **Fallback: `gpt-5-mini`** (same endpoint, same schema). More robust on the hard cases, 3x the price and 2-3x the latency (reasoning tokens). Use it for the retry after a failed or invalid first call, and as the pick if the Owner would rather spend $0.17/month for fewer misses.
- **Not recommended:** `gpt-4.1-nano` (cheapest sane option, but reads the balance as the amount when the amount line is missing, case 13, and drops dates), `gemini-2.5-flash-lite` (wrong date on 5 of 18 cases, and not reachable on the OpenAI route), `gpt-5-nano` (15 s median latency, 7 of 54 calls dropped).
- Price is no reason to use a nano model: the whole bot costs cents either way.

## Setup

- 18 cases (the `expect:` lines stripped), 3 repeats per model = 54 calls per model, `temperature: 0` where supported (gpt-5 models reject it and use their default), one Input per call, sequential per model, models run in parallel.
- Scored per call, per field, against the sample's expected result converted to toman: amount, direction, date, and description (the Owner's note verbatim, or null when there is none). Case 13 (amount line missing) expects `null` and direction out or ambiguous. Case 16 expects two entries, case 8 one `internal` entry (prompt v1) or two entries with opposite signs (prompt v2).
- Prompt v1 follows the contract in #10 (`internal` for an equal +/- pair). Prompt v2 changed three lines after reading the v1 failures (see below). The model returns `amount` as printed plus `amount_unit`; code divides rial by 10, so no digit or unit conversion depends on the model.
- Whole experiment: roughly 700 calls, about $0.25-0.30 of Metis credit.

## Results

Prompt v1, `json_schema` mode, 54 calls per model. Counts are correct calls out of 54; failed calls (network errors) count as wrong.

| Model | Amount | Direction | Date | Description | Failed calls | Cost/call | Latency p50 / p95 |
|---|---|---|---|---|---|---|---|
| gpt-4.1-mini | 51 | 51 | 51 | 48 | 0 | $0.00037 | 2.6 s / 4.9 s |
| gpt-5-mini | 50 | 50 | 50 | 47 | 4 | $0.00116 | 6.3 s / 10.3 s |
| gpt-4.1-nano | 48 | 48 | 48 | 45 | 0 | $0.00009 | 2.8 s / 4.6 s |
| gpt-5-nano | 46 | 47 | 46 | 44 | 7 | $0.00082 | 15.6 s / 22.7 s |
| gemini-2.5-flash-lite (native route) | 48 | 51 | 39 | 51 | 0 | $0.00010 | 2.5 s / 4.0 s |

Cost uses actual `usage` token counts from each response and the Metis prices: about 600 input tokens per call (prompt about 500, Input about 100) and 50 output tokens for the non-reasoning models. gpt-5-nano emits about 1800 and gpt-5-mini about 450 output tokens a call, mostly hidden reasoning, which is why gpt-5-nano costs 9x gpt-4.1-nano and is slow.

Prompt v2 (3 repeats), `gpt-4.1-mini`: on the 51 calls that returned, amount, direction and date were 51/51; description 48/51 (only case 16). `gpt-4.1-nano` v2: amount 51/54, date 45/54. `gpt-5-mini` v2: amount, direction, date 50/50 returned; description 44/50 (it sometimes drops a typo-bearing note, cases 1, 2, 10, 13). Failed calls: 3, 0 and 4.

Failure patterns by case:

- **Case 8 (equal +/- pair as one `internal`)**: `gpt-4.1-mini` v1 returned two `internal` entries (0/3), gemini returned out and in with a wrong date, only `gpt-5-mini` got it. Fix: do not ask the model to merge. Prompt v2 says to return one entry per message with its own sign, and code merges an equal +/- pair at the same minute into one `internal` Transaction. `gpt-4.1-mini` is then correct on 8.
- **Case 13 (no amount line, only a balance)**: `gpt-4.1-nano` and gemini return the balance as the amount (silent wrong data; the code check "number appears in the text" does not catch this, because the balance is in the text). `gpt-4.1-mini` and `gpt-5-mini` return null. Keep a model that returns null here, and keep the prompt line saying so.
- **Case 16 (one note, two messages)**: `gpt-4.1-mini` and nano attach the note to the last entry only. Fix: make the note one top-level `note` field and let code copy it to every entry (not tested, but removes the failure by construction).
- **Dates**: gemini fills the year-less SMS dates (cases 6, 10, 12) and several others with today's date instead of the message date. `gpt-4.1-nano` fails case 6 on the same year-less format. The mini models parse the year-less `MMDD-HH:MM` format correctly in every call.
- **Case 7 (hashtag says transfer, sign is +)**: `gpt-4.1-nano` misses direction; mini models correct.
- **Cases 18 and 14 (hashtag contradicting sign; note-only toman amount)**: `gpt-4.1-mini` passed all repeats.
- Notes with typos are copied verbatim by `gpt-4.1-mini` in every case but 16; `gpt-5-mini` occasionally returns null for such notes.

## Structured-output reliability

- **JSON mode works on Metis** for `gpt-4.1-mini`, `gpt-4.1-nano`, `gpt-5-nano`, `gpt-5-mini`: `response_format: {"type":"json_object"}` accepted (200). `json_schema` strict also accepted and followed.
- Across all runs no response was malformed JSON or off-schema: every call that returned HTTP 200 parsed and matched. Without any `response_format` (prompt only), `gpt-4.1-mini` was also valid 36/36 and equally accurate (36/36 on amount, direction, date). `json_schema` is still the right choice: it costs nothing and guarantees the shape.
- What did fail was the transport: `RemoteDisconnected` ("remote end closed connection without response") or an HTTP 503 "upstream connect error" on about 5-13% of calls for gpt-5-mini, gpt-5-nano, and some gpt-4.1 runs, mostly when three models were run in parallel from a new connection each call. No such errors in the first sequential smoke test. The contract's "retry once, then Flagged" is warranted; make the retry immediate and the client reuse connections.
- Gemini works only on the native route (below), where `responseMimeType: application/json` produced valid JSON on 54/54 calls.
- Repeat runs: with `temperature: 0` `gpt-4.1-mini` gave the same result on all 3 repeats for every case except the failed calls (no flapping). gpt-5-mini missed single repeats on cases 6, 13, 14, 15 (mostly dropped calls, so not clearly flapping).

## Open items from `metis-api.md`

| Item | Result |
|---|---|
| JSON mode / structured outputs | Works for the OpenAI models, both `json_object` and strict `json_schema` (above). |
| Non-OpenAI models on `/openai/v1/chat/completions` | `gemini-2.5-flash-lite` returns 404 `model_not_found` there (also `google/...` and `gemini-2.0-flash-lite` are refused). It works on the Gemini-native route `POST https://api.metisai.ir/v1beta/models/<model>:generateContent` with header `x-goog-api-key`. Using it would need a second request/response shape. |
| Key format | Plain `Authorization: Bearer <key>` works on both OpenAI and Gemini routes (Gemini via `x-goog-api-key`). |
| Rate limits | Response headers on OpenAI routes: `x-ratelimit-limit-requests: 30000`, and 150,000,000 tokens (gpt-4.1 family) or 180,000,000 tokens (gpt-5 family) per window. Orders of magnitude above this bot's use; the window length is not stated and the reset header read `2ms`. Not an issue. |
| Billing unit / USD-to-toman | Not verified. The `usage` object carries token counts only (including `reasoning_tokens`, which are billed as output) and no cost; I did not find a balance endpoint. Cost figures above are token counts times the listed USD prices. |
| Reasoning-token billing | gpt-5 family: `usage.completion_tokens` includes hidden reasoning (128 of 142 tokens on a trivial prompt), so output cost is far above what the visible JSON suggests. This is the cost trap of the gpt-5 family. |

## Monthly cost

Assumption: 5 Inputs a day = 150 a month, one call each, 20% retried (180 calls), about 600 input and 50 output tokens (a longer 5-message Input costs about 3x).

| Model | $/month at 150 calls | at 20 Inputs/day |
|---|---|---|
| gpt-4.1-nano | $0.017 | $0.07 |
| gemini-2.5-flash-lite | $0.019 | $0.08 |
| gpt-4.1-mini | $0.067 | $0.27 |
| gpt-5-nano | $0.15 | $0.59 |
| gpt-5-mini | $0.21 | $0.84 |

A Jev call for the Category step adds roughly $0.00002 per Input (from `metis-api.md`), negligible. Extraction is the largest LLM cost and it is still under a cent a week.

## Prompt shape

One system message, one user message containing the whole Input verbatim (bank messages plus note, before or after), no few-shot examples needed (the zero-shot prompt below scored as reported). Input tokens about 500 for the system message.

System message contents, in order:
1. Role and input description: forwarded Persian bank messages, optional user note (before or after), digits in any script.
2. Today's Jalali date (`Asia/Tehran`), injected per call.
3. The output JSON shape, then the rules, one bullet each: one entry per bank message and `is_transaction=false` for non-transactions; amount as printed in ASCII digits with the sign dropped, never the balance, null if no amount line; unit rial for any bank message and toman only for a note-only amount; direction by sign over hashtag over keyword over note, a bare transfer word is `ambiguous`; never output `internal` (code merges an equal +/- pair at the same minute); date as printed, year-less `MMDD-HH:MM` means the current year, never invent one; note copied verbatim, typos kept.

Output schema (strict `json_schema`, `additionalProperties: false`):

```
{ "is_transaction": bool,
  "note": string|null,                         // top-level, code copies it to every entry
  "transactions": [ { "amount": integer|null,  // as printed, ASCII digits
                      "amount_unit": "rial"|"toman",
                      "direction": "out"|"in"|"ambiguous",
                      "date": "YYYY/MM/DD"|null, "time": "HH:MM"|null,   // time lets code detect the pair
                      "confidence": number } ] }
```

Changes from the contract's schema, all tested except where marked: the model returns amount plus unit instead of toman (code does the division, as the contract already says); `internal` is derived by code, not by the model; `description` moves to a top-level `note` and `time` is added (neither tested: the tested schema kept `description` per entry and had no `time`). These are small and follow from the failures above; one more run on the 18 cases should confirm them before building.

Code-side checks stay as in #10: amount must appear in the text, retry once on failure or invalid JSON, then Flagged. Add one: reject an amount that equals the printed balance when the message has no amount line (cheap guard if a nano model is ever used).

## Caveats

- 18 cases, one bank (Melli) in two formats plus one note-only case; other banks are not tested. Scoring is exact match against expected results, and sample 16 and 13 expectations are the Owner's drafts. Descriptions are scored by containment of the expected note, not strict equality.
- Prompt v2 was tuned on the same 18 cases it was measured on, so its numbers are optimistic; v1 numbers are the unbiased comparison between models.
- Latency was measured from a home connection to Metis; the lab network may differ. Metis serving errors came in bursts and may be partly caused by my parallel test runs.
- Persian quality beyond these cases (long free-text notes, multiple notes, dates like «دیروز») is unmeasured.
