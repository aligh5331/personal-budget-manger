# Jev via v1m: classification API, pricing, Persian support

Research for ticket #4 (map #1). Researched 2026-10-05. Scope: only the Category step (short Persian transaction description to a fixed Category list). Jev does no STT, no free text, no amount extraction.

## Verdict

- **Technically fit, unproven for Persian.** Jev's Choice question is built for "pick one of N labels" (up to 255 options). It returns a probability per option plus a confidence, so a low-confidence result can be routed to a Flagged transaction.
- **Persian is the risk.** TypeSafe says English is the primary training language, other languages are "not equally well" handled, and you should test on your own content. Nothing first-party measures Persian. v1m's Persian accuracy claims are marketing, not evidence.
- **v1m adds little and costs trust.** Its docs contradict TypeSafe's schema (see "Doc mismatches"), it publishes no toman price, and its stated Jev price is far off TypeSafe's own. If Jev is used, calling TypeSafe directly is the safer default. Treat v1m as unproven until a key is tested.
- **No Go SDK exists.** Only Python and JS/TS SDKs. Plain `net/http` with one JSON POST is enough.
- **Cost is negligible either way** (see Pricing).
- **Blocked on the Owner's sample data** to judge accuracy, and on a real key: neither provider's key was available here.

## Request and response shape

Same endpoint path on both: `POST /v1/systemone`, `Authorization: Bearer <key>`, JSON.

| | TypeSafe | v1m |
|---|---|---|
| Base | `https://api.typesafe.ai/v1/systemone` | `https://v1m.ir/v1/systemone` |
| Key prefix | not stated | `v1m_live_` |
| Model name | `jev-latest` (alias for `jev-1.13.0`) | `v1m-latest` (docs); OpenAPI default is `rev-latest`; `jev-1.13.0` also listed |

Sources: https://docs.typesafe.ai/api.md, https://docs.typesafe.ai/introduction/quickstart.md, https://docs.typesafe.ai/models.md, https://v1m.ir/docs, https://v1m.ir/openapi.json.

### Choice request (TypeSafe schema, also what v1m's OpenAPI enforces)

```json
{
  "model": "jev-latest",
  "state": "خرید از دیجی کالا ۴۵۰۰۰۰ تومان",
  "questions": {
    "category": {
      "type": "choice",
      "instructions": "Which spending category does this bank transaction belong to?",
      "criteria": {
        "groceries": "Supermarkets, food shopping",
        "transport": "Taxi, fuel, metro, parking",
        "shopping": "Online and offline retail"
      }
    }
  }
}
```

- `instructions` is required. For a Choice, `criteria` is a map of option name to description (null allowed). Max 255 options.
- `state` may be a string, object or array.
- Response (TypeSafe docs):

```json
{"model":"jev-1.13.0",
 "answers":{"category":{"type":"choice","choice":"shopping",
   "probabilities":{"shopping":0.85,"groceries":0.15,"transport":0.0},
   "confidence":0.78}},
 "usage":{"input_tokens":318,"output_tokens":34}}
```

- `confidence` is derived from the distribution. TypeSafe recommends gating on it (https://docs.typesafe.ai/confidence.md, pattern "Confidence-gated routing"). Use it for the "unsure, flag it" path, and include an explicit "other/unknown" option.
- Many questions can share one `state` in one request, but the Category step needs only one.

### Doc mismatches

- v1m's docs page shows Choice as `"choices": ["a","b"]` with no `instructions`. v1m's own OpenAPI (`Question` schema: `type` and `instructions` required, `criteria` optional, no `choices`) disagrees. A live unauthenticated POST with that shape returned **422, `questions.a.instructions` Field required**. Use `instructions` + `criteria` (the TypeSafe shape; v1m's own Python and JS examples also use it).
- v1m's docs show the Choice response with `confidence` + `distribution`. TypeSafe returns `probabilities`. Unverified which v1m actually returns, because the 200 response schema in its OpenAPI is empty. Parse defensively or test with a key.
- v1m's docs page contradicts itself on limits: 120 requests/minute in the prose, 600 RPM in the dashboard block and the page script. The active-model list also differs between sections ("v1m-latest, v1m-1.0" vs four models).
- v1m claims "100% compatible" with Jev, "change base URL only". Only partly true given the above.

## Pricing

- **TypeSafe (first-party):** Jev 1.13 costs $42 per billion input tokens ($0.042 per million). Output tokens are free. (https://docs.typesafe.ai/models.md, https://typesafe.ai)
- **Rough cost for this bot:** a request with a short Persian description plus about 15 Category descriptions is on the order of 400 to 1,000 input tokens (an estimate; Persian tokenization on Jev is not documented). That is about $0.00002 to $0.00004 per Input. Even 100 Inputs a day is well under a cent a month. No toman conversion done, since no authoritative rate was used.
- **v1m:** claims "50% cheaper than Jev", $10 free credit (vs $5 on Jev), 500 free requests per day (resets 00:00 UTC), payment in USD crypto (OxaPay) or Iranian rial via Shaparak. **No price list and no toman figure is published** in the docs page, the OpenAPI or the other pages checked. v1m's comparison table puts Jev at "~$15 / 1k" decisions, roughly 900 times TypeSafe's own price, so its "half of Jev" figure cannot be taken at face value. (https://v1m.ir/docs)
- 500 free requests per day exceeds the Owner's expected volume, so v1m could be free in practice. Whether that tier lasts is a marketing claim ("free forever"), not a stated term.

## Latency and limits

- TypeSafe publishes no latency figure. Jev 1.13 limits: 80 requests per second, 100K tokens per second, 64k context (32k for state plus the longest question). TypeSafe says limits "adjust dynamically" without notice. 429 and 529 are retryable.
- v1m claims under 5 ms cached and under 50 ms cold, and about 420 ms for upstream Jev. Self-reported and not measured here. It barely matters for a bot that waits on STT first.
- Errors v1m documents: 400, 401, 429, 500. Its OpenAPI shows 422 for validation.

## Persian support

- **TypeSafe:** "English is the primary training language and where accuracy is currently best. Other languages, including CJK scripts, are handled but not equally well; test on your own content." Persian is not mentioned. Jev is also literal, weak on numbers and dates, and option order can bias the result (reorder options to check). (https://docs.typesafe.ai/models.md, https://docs.typesafe.ai/model-jaggedness/jev-1.13.md)
- **v1m:** claims Persian-tuned models: `qwen2.5-1.5b-systemone` ("fine-tuned on 2,100 Persian legal/banking/fintech scenarios") and `laya-multilingual-onnx` (FA/AR/EN). The live `/api/training/stats` endpoint reports 63 scenarios and 7,995 words, which does not match "2,100". These are different models from Jev, so choosing one means not using Jev. No accuracy numbers anywhere. Its examples send Persian state, instructions and option names, so Persian text is accepted.
- Mitigations if tried: test Category descriptions in English and in Persian; keep option keys ASCII; gate on confidence; strip amount and date from the state in code before sending (Jev is weak on numbers).

## Go SDK

None. SDKs are Python (`typesafe-sdk`) and JS/TS (`@typesafe-ai/sdk`). v1m's "Go" tab is a short `net/http` snippet, not a package. Call the HTTP API directly. Sources: https://docs.typesafe.ai/sdk.md, https://v1m.ir/docs.

## Other risks

- v1m is a third-party service with an unclear relationship to TypeSafe. Bank SMS text would be sent to it. TypeSafe documents zero data retention only for enterprise customers; no v1m privacy terms were found.
- v1m's claims of "immune to sanctions" and "99.9% uptime" are unverified.
- Whether TypeSafe's overseas API is reachable and payable from Iran is not documented. The lab can test.
- Adversarial text in an SMS could steer the answer (TypeSafe notes this). Low stakes for a personal budget bot.

## Not verified

- Actual Persian accuracy on Jev, `v1m-latest`, or the two Persian models. Needs a key plus the Owner's sample SMS and voice-note transcripts.
- v1m's real response field names, and whether Choice with `criteria` works on all its models.
- v1m's price per decision (toman or USD), whether the free tier persists, and the true rate limit (120 vs 600).
- Persian token count per request.
- Whether TypeSafe can be paid for from Iran.
- Neither signup flow (Google/GitHub login) was exercised.
