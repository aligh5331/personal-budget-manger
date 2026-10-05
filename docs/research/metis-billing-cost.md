# Metis billing unit and v1 monthly cost

Research for ticket #26 (map #1). Retrieved 2026-10-06.

Sources:
- Metis docs, quick start, section «شارژ حساب و محاسبه‌ی هزینه‌ها»: `https://docs.metisai.ir/start/quick-start/` (plain `curl` works). Also `/introduction/` and `/api/wrapper/openai/`.
- Metis blog post «استفاده از APIهای هوش مصنوعی» (08 Bahman 1403, about January 2025), section «پرداخت ریالی». `metisai.ir` times out from this network, so I read the Wayback copy: `http://web.archive.org/web/20250702110028/https://metisai.ir/%D8%A7%D8%B3%D8%AA%D9%81%D8%A7%D8%AF%D9%87-%D8%A7%D8%B2-api%D9%87%D8%A7%DB%8C-%D9%87%D9%88%D8%B4-%D9%85%D8%B5%D9%86%D9%88%D8%B9%DB%8C/`. (The archived `metisai.ir/pricing` page is placeholder template text, so it is useless.)
- Pricing JSON behind https://docs.metisai.ir/pricing: `GET https://api.metisai.ir/api/v1/meta/providers/pricing` (public, see `metis-api.md`).
- Usage endpoint the Metis console uses (found in the console JS, `console.metisai.ir/_next/static/chunks/app/dashboard/page-*.js`): `GET https://api.metisai.ir/api/v1/statistica/overview?startDate=<ms>&endDate=<ms>`. It accepts the API key as Bearer and returns cost per day, per provider/model, per API key, with a per-token breakdown.
- My own measurements: two tiny live calls (one Jev, one `gpt-4.1-mini`, total $0.00002), with `statistica` read before and after. Also token counts from `extraction-llm.md`, `jev-sole-call.md` and `jev-breadth.md`.
- USD to toman market rate: Hamshahri Online, 13 Mehr 1405 (2026-10-05): free-market dollar about 269,080 toman. https://www.hamshahrionline.ir/news/1074222

## Answer

- **Unit: USD, prepaid, pay as you go, no per-request minimum or rounding.** Every price in the pricing JSON is `currency: USD`, and `statistica` reports spend in USD to about 1e-11 precision. Two test calls were charged exactly tokens times list price: Jev 311 input tokens = $0.0000143682 (311 x $0.0462/1M, output free), `gpt-4.1-mini` 9 in + 1 out = $0.00000572. Both showed up in `statistica` within 5 seconds. The model entries carry `fixedCallIncome: 0` (some image models have a per-call fee, the chat models and Jev do not).
- **Markup: list price is the official model price plus 10%.** The quick start says «اساس محاسبه‌های هزینه بر اساس قیمت‌های رسمی مدل‌ها می‌باشد به‌علاوه‌ی ۱۰٪ مازاد هزینه‌ی متیس». The pricing JSON already includes it (`gpt-4.1-mini` $0.44/1M input = OpenAI's $0.40 + 10%). So the per-token prices in `metis-api.md` are what you pay, in USD.
- **Toman: you top up in rial; the panel balance is in dollars.** The blog says the panel is charged in dollars and the rial equivalent is moved to your account, and the cost is "charge amount + dollar-to-rial conversion cost", with the breakdown shown at top-up time («شارژ پنل متیس نیز بر اساس دلار می‌باشد و معادل ریالی آن به حساب کاربری شما منتقل می‌شود. نحوه محاسبه‌ی هزینه در متیس برابر است با: مقدار شارژ حساب + هزینه‌ی تبدیل دلار به ریال»). The introduction lists «امکان پرداخت ریالی» and «Pay-as-You-Go». **The rate Metis uses and the size of the conversion fee are not published** anywhere I could reach.
- **Prepaid credit:** new accounts get $0.50 gift credit (quick start: «به میزان نیم دلار اعتبار هدیه»). Usage is deducted from the balance in real time, and on some heavy jobs the balance can go negative.
- **Minimum top-up: not found.** It is only visible on the logged-in dashboard, which I could not reach.
- **v1 monthly cost at 5 Inputs/day: about $0.08 expected (low $0.05, worst case $0.62), about 21,000 toman expected (13,000 to 170,000)** at the free-market rate, before Metis's unknown conversion fee. The $0.50 gift credit alone covers about six months at the expected rate.

## Cost model

Volume: 5 Inputs/day = 150 a month. Prices: Metis list (USD per 1M tokens), `gpt-4.1-mini` 0.44 in / 1.76 out, `gpt-5-mini` 0.275 in / 2.20 out (hidden reasoning tokens are billed as output), `jev-latest` 0.0462 in / 0 out.

Per-call inputs, from the measured runs:
- `gpt-4.1-mini` extraction: about 600 in, 50 out per call, $0.00035 (`extraction-llm.md` measured $0.00037). A multi-message Input (up to 5 bank messages) is about 3x.
- `gpt-5-mini` retry: about 600 in, 450 out (mostly reasoning), $0.00116 measured.
- Jev Category call: the four-question Jev calls measured 1,350 to 1,630 input tokens; my two-option test call was 311, so the fixed overhead is about 300 and the rest is option text. A Category-only call with 15 Categories plus Persian hints should land around 600 to 1,000 (estimate, not measured). At 900 tokens it is $0.00004.

| Scenario | Assumptions per Input | $/Input | $/month (150) | Toman/month at 269,080 |
|---|---|---|---|---|
| Low | short Input (500 in, 40 out), no retries, Jev 500 tokens, one call each | $0.00031 | $0.047 | about 12,700 |
| Expected | 600 in / 50 out; 10% of Inputs retried on `gpt-5-mini`; Jev 900 tokens plus 15% extra Jev calls for retries and the re-categorize queue | $0.00052 | $0.077 | about 20,800 |
| High (worst case) | every Input is a 3x multi-message Input, every first extraction call is billed and then retried on `gpt-5-mini` (1,800 in, 900 out), every Jev call fails and is billed 8 times (1 + 1 retry + 6 queue tries) at 1,600 tokens | $0.0041 | $0.62 | about 166,000 |

At 20 Inputs/day multiply by 4: expected about $0.31, worst case about $2.50.

Notes on the model:
- Extraction dominates. In the expected case `gpt-4.1-mini` is about 68% of the cost, the `gpt-5-mini` retries 22%, Jev under 10%.
- The worst case is pessimistic on purpose: it charges failed calls in full. A dropped connection or a 503 "no healthy upstream" most likely bills nothing, since no tokens were produced (not verified). Then the failed attempts cost nothing and the high case falls to about $0.38 a month (every Input a multi-message Input extracted by `gpt-5-mini`, plus one successful Jev call).
- The re-categorize queue only runs after a Jev failure, max 6 tries, at $0.00004 to $0.00007 per try. Even if every Input used all 6 tries and each was billed, it would add $0.04 to $0.07 a month.
- Measured failure rates for scale: `gpt-4.1-mini` had 0 dropped calls in sequential runs, 3 of 54 in a parallel run; `gpt-5-mini` 4 of 54; Jev 0 of 54 in #17, but a one-minute Metis outage hit 88 of 90 calls in #21.
- Toman figures use the free-market rate on 2026-10-05; add Metis's conversion fee (unknown, the table's toman column x 1.1 if it were 10%). The rial cost moves with the dollar.

## Monitoring spend

- `GET /api/v1/statistica/overview?startDate=<ms>&endDate=<ms>` with the API key returns daily cost by model and by API key name (the bot's key, "personal budeg manager", shows separately from the Owner's other keys on the same account). The bot or the Owner can use it to check spend. It is the console's internal endpoint, not documented, so it may change.
- No balance endpoint found. `/api/v1/wallet`, `/api/v1/balance`, `/api/v1/user`, `/api/v1/users/me` return 404. `/openai/v1/dashboard/billing/credit_grants` returns OpenAI's "session key only" 403 (passed through from upstream).
- No cost or balance headers on responses: the Jev response has only standard security headers; the OpenAI route passes through OpenAI's rate-limit headers (`x-ratelimit-limit-requests: 30000`).
- The account balance is shared across all the Owner's keys, so another project can drain it. A low balance makes every call fail. Unverified what Metis returns then (likely 402 or 403); the bot should treat it like any other failure (Flagged, re-categorize queue) and tell the Owner.

## Not verified

- The USD-to-rial rate Metis uses at top-up, and the conversion fee. Both are shown on the dashboard's top-up screen, which I could not reach (`metisai.ir` times out from here). The Owner can read them on the next top-up.
- Minimum top-up amount.
- Whether failed calls (503, dropped connection, timeouts) are billed. The likely answer is no, since no `usage` is returned.
- The error code when the balance runs out, and whether calls are cut off at zero or allowed slightly negative (the docs say only heavy jobs can go negative).
- Token count of a Category-only Jev call (estimated 600 to 1,000 from the four-question measurements).
- Whether the 2025 blog's description of rial top-up still holds in October 2026. The 10% markup and $0.50 gift come from the current docs.
