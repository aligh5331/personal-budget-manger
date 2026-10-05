# Iranian bank deposit SMS formats (ticket #6)

Question: what do deposit and withdrawal SMS from major Iranian banks look like (rial vs toman, Persian digits, Jalali date, balance, counterparty)?

## Short answer

No bank publishes a spec for its transaction SMS. The only public evidence is (a) bank pages that say *that* SMS exist and (b) open-source SMS parsers whose test fixtures encode developers' observations. Treat everything below as **candidate patterns, not verified real messages**. The Owner's own samples (separate ticket) are the ground truth.

## What is established

1. **Bank Melli confirms transaction SMS cover deposits, withdrawals and balance**, and says transactions under 30,000 toman stopped being sent by SMS from 23 March 2022, with notices moved to Bale, Neshan, BAM and phone banking. Source: https://bank-melli.ir/landing/sms (fetched via summarizer; the page gives no sample text). Consequence: small deposits may never arrive as SMS.
2. **Bank Melli has a Bale channel for notifications** (same source). Out of scope for v1 but worth a later look.
3. **No official sample text** was found for Melli. The Mellat SMS-bank page (https://www.bankmellat.ir/smsbank.aspx) could not be fetched (TLS certificate error: host not in `*.mellat.ir`); a search snippet only shows it describes *request* commands (password + account + `11` to 15560026 for balance and last three transactions), not notification text.

## Observed structure (from open-source parser fixtures)

Sources, all public GitHub, all developer-written fixtures (not captured-and-attributed real SMS):

- masein/accounting-assistant-web, `tests/test_bank_sms.py` (PR #156): https://github.com/masein/accounting-assistant-web/pull/156 . PR text calls the eight formats "real-world"; the repo does not say they are verbatim copies.
- milibots/smsGAClient, `app/src/main/assets/banks/*.json` (`sample_sms` fields): https://github.com/milibots/smsGAClient . Their Melli/Mellat/Saman samples have round numbers and fake-looking tracking numbers, and use the `۶٬۲۰۰٬۰۰۰ ریال` style; they look synthetic.
- ebrahimpersianh/Bank, `BankSmsParser.kt` and its test: https://github.com/ebrahimpersianh/Bank . Comments cite user-reported false positives (bills, ads, future-tense notices, dynamic passwords), which is the most useful real-world signal here.

### Shape of a typical message (multi-line, key:value)

Fixtures from masein (one per bank, as written in the repo):

```
Mellat   : بانک ملت\nبرداشت:125,000\nحساب:123456789\nمانده:1,234,567\n0703-13:45
Mellat 2 : بانک ملت\nواریز:65,433+\nحساب:123456789\nمانده:1,300,000\n0704-09:10
Melli    : بانك ملي ايران\nواريز:2,500,000\nاز 0123456789\nمانده:9,876,543\n05/07/02_12:30
Saman    : بانک سامان\nبرداشت از حساب 849-800-1234-1\nمبلغ: 150,000 ریال\nمانده: 3,000,000\n1405/07/01 - 18:20
Tejarat  : بانك تجارت\nحساب:0123456\nبرداشت(خريد):250,000\nمانده:1,000,000\n1405/07/03-09:10
Pasargad : بانک پاسارگاد\nانتقال\nمبلغ:1,000,000-\nحساب:201.8000.1234.1\nمانده:5,000,000\n07/02 10:00
Ayandeh  : بانک آینده واریز ۵۰۰٬۰۰۰ تومان به حساب ۰۲۰۱۲۳۴۵۶۷ مانده ۱٬۲۰۰٬۰۰۰ تومان ۱۴۰۵/۰۷/۰۳
Saderat  : بانک صادرات\nخرید کارت 6037****1234\nمبلغ 85,000-\nمانده 2,000,000\n1405/07/04 20:15
```

smsGAClient fixtures differ (`واریز: ۶٬۲۰۰٬۰۰۰ ریال`, `کارت: ۵۶۷۸`, `موجودی: ...`, a `پیگیری` tracking line) and show Persian digits with `٬` separators plus an explicit `ریال` suffix.

### Patterns the fixtures agree on

| Aspect | Observed variation |
|---|---|
| Unit | Often **no unit at all** (bare `125,000`); sometimes `ریال`; sometimes `تومان` (Ayandeh fixture). Ambiguity is real: the parser must default per bank and convert (toman x 10 = rial). Owner wants whole toman, so decide the default unit per bank from the Owner's samples. |
| Digits | ASCII, Persian (`۰-۹`) and Arabic-Indic all appear; separators `,` and `٬`. Arabic letters `ي` `ك` appear instead of Persian `ی` `ک` (Melli, Tejarat fixtures). Normalize before matching. |
| Direction | Keyword (`واریز`, `برداشت`, `خرید`, `پرداخت`, `انتقال`) and/or a trailing sign (`65,433+`, `85,000-`). `انتقال` alone is ambiguous (Refah fixture is rejected for that reason). |
| Balance | After `مانده` or `موجودی`; same unit rules as amount. |
| Account/card | Masked or partial (`6037****1234`, last 4 digits, dotted/dashed deposit numbers). |
| Date/time | Jalali, **often year-less and compact**: `0703-13:45`, `05/07/02_12:30`, `07/02 10:00`, sometimes full `1405/07/03`; sometimes absent. Year must be inferred (masein picks the most recent past date). Time is local, no seconds. |
| Counterparty/merchant | **Not present** in any fixture. Deposits do not name the sender; purchases give card, not merchant. Do not design the contract around a merchant field coming from SMS. |
| Sender short code | smsGAClient lists sender names/numbers per bank (e.g. `MELLAT`, `BMI`); unverified and may be stale. |

### Messages that look like transactions but are not (from ebrahimpersianh/Bank, cited as real user reports)

Bill notices ("قابل پرداخت", "مهلت", "شناسه قبض"), operator ads (internet packages), BNPL credit returns (Digipay/SnappPay), dynamic-password SMS (contain an amount but no money moved), and future-tense notices ("واریز خواهد شد"). Bank-grade reports nearly always carry a balance line. Matches the Flagged transaction idea: when a message has an amount but no balance/account/past-tense evidence, flag instead of record.

## Implications for the Input-to-Transaction contract

- Normalize text first (digits, `ي/ك`, bidi marks, separators), then extract.
- Amount unit is a per-message decision (explicit unit, else bank default); store whole toman.
- Date is partial: contract needs a "date missing or year inferred" case; fall back to message receive time.
- Sender and merchant cannot be relied on; category must come from the Owner or from the free-text note, not the SMS.
- Expect non-transaction SMS to be pasted/forwarded; rejecting or flagging them is part of the job.
- Whether to use an LLM for extraction or regexes per bank is a decision for the ticket that owns it; this note only supplies the variation to cover.

## Could not verify

- That any fixture above is a verbatim real SMS from the named bank.
- Current (2026) wording; banks change formats and fixtures date from various times.
- Melli/Mellat/Saman real notification text (no official sample found; Mellat page unreachable).
- Whether Bank Melli's under-30,000-toman cutoff still applies and whether other banks have similar cutoffs.
- A Shikoonet-Platform issue (#417) surfaced in search with a Melli-style line (`انتقالی:461,200- | حساب:06006 | مانده:96,270...`) but the repo returns 404 now, so it is not used above.
- Banks not covered: Blu, Sepah, Parsian, Keshavarzi and others have only parser metadata, no usable samples read.
