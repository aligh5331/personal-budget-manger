"""Throwaway: re-run the 18 sample cases with the final extraction prompt, then Jev for Category. Ticket #27.
Pipeline as decided in #10, #12, #13, #14:
  gpt-4.1-mini (strict json_schema, temperature 0) -> amount as printed + unit, direction out/in/ambiguous,
  per-entry date+time, one top-level note -> code: rial/10, merge equal +/- pair at same minute into internal,
  copy note to every entry -> Jev picks Category among the Direction's kind + Uncategorized, < 0.7 = Uncategorized.
Usage: python3 extraction_rerun.py <samples file> <env file> [runs]
"""
import json, re, sys, time, urllib.request, urllib.error

BASE = "https://api.metisai.ir"
SAMPLES, ENVF = sys.argv[1], sys.argv[2]
RUNS = int(sys.argv[3]) if len(sys.argv) > 3 else 3
TODAY = "1405/07/13"  # the samples were written on this day; case 14 expects it
P_IN, P_OUT = 0.44 / 1e6, 1.76 / 1e6  # gpt-4.1-mini on Metis, USD per token (metis-api.md, incl. +10%)
JEV_IN = 0.046 / 1e6

KEY = None
for line in open(ENVF, encoding="utf-8"):
    if line.startswith("LLM_API_KEY="):
        KEY = line.split("=", 1)[1].strip()

SYSTEM = f"""You read Persian bank messages that the user forwarded to a budgeting bot, plus an optional short note the user typed (usually after the messages, sometimes before). Digits may be ASCII, Persian or Arabic-Indic. Today is {TODAY} (Jalali, Asia/Tehran).

Return JSON with this shape:
- is_transaction: false if the input is not a money movement at all (ad, bill notice, future-tense notice, chit-chat), else true.
- note: the user's own note copied verbatim (typos kept, not translated, not corrected), or null if there is none. Bank message text is never the note.
- transactions: one entry per bank message; if there is no bank message but the note states a payment, one entry from the note.
  - amount: the amount moved, as printed, in ASCII digits, sign and separators dropped. Never the balance (مانده). null if the message has no amount line.
  - amount_unit: "rial" for any amount read from a bank message; "toman" only for an amount stated in the user's note when there is no bank message.
  - direction: "out" or "in". The sign printed after the amount decides first (- is out, + is in), then the hashtag or keyword (برداشت, خرید = out; واریز = in), then the note. A bare transfer word (انتقال) with no sign is "ambiguous". Never output anything else.
  - date: Jalali "YYYY/MM/DD" as printed. A year-less "MMDD-HH:MM" means the current year. If the note gives a day (امروز = today), use it. Otherwise null; never invent a date.
  - time: "HH:MM" as printed, or null.
  - confidence: 0..1, how sure you are about amount and direction."""

SCHEMA = {
    "name": "input_extraction", "strict": True,
    "schema": {
        "type": "object", "additionalProperties": False,
        "required": ["is_transaction", "note", "transactions"],
        "properties": {
            "is_transaction": {"type": "boolean"},
            "note": {"type": ["string", "null"]},
            "transactions": {"type": "array", "items": {
                "type": "object", "additionalProperties": False,
                "required": ["amount", "amount_unit", "direction", "date", "time", "confidence"],
                "properties": {
                    "amount": {"type": ["integer", "null"]},
                    "amount_unit": {"type": "string", "enum": ["rial", "toman"]},
                    "direction": {"type": "string", "enum": ["out", "in", "ambiguous"]},
                    "date": {"type": ["string", "null"]},
                    "time": {"type": ["string", "null"]},
                    "confidence": {"type": "number"},
                }}},
        }},
}

# Starting list from #14: key -> (name, persian hint, kind)
CATS = {
    "food": ("Food", "غذا، ناهار، شام، رستوران", "expense"),
    "snacks": ("Snacks", "تنقلات، خوراکی، شیرینی، کافه", "expense"),
    "groceries": ("Groceries", "نان، سوپرمارکت، میوه، خرید خانه", "expense"),
    "transport": ("Transport", "تاکسی، اسنپ، مترو، اتوبوس", "expense"),
    "fuel": ("Fuel", "بنزین، سوخت", "expense"),
    "education": ("Education", "کلاس، دوره، کتاب، دانشگاه", "expense"),
    "entertainment": ("Entertainment", "سینما، نتفلیکس، اشتراک، تفریح", "expense"),
    "health": ("Health", "داروخانه، دکتر، دارو، درمان", "expense"),
    "bills": ("Bills", "قبض برق، آب، گاز، موبایل، اینترنت", "expense"),
    "loan_installment": ("Loan installment", "قسط، وام", "expense"),
    "shopping": ("Shopping", "لباس، دیجی‌کالا، خرید", "expense"),
    "cash_withdrawal": ("Cash withdrawal", "برداشت نقدی", "expense"),
    "rent": ("Rent", "اجاره", "expense"),
    "gifts": ("Gifts", "هدیه، کادو", "expense"),
    "other": ("Other", "سایر", "expense"),
    "salary": ("Salary", "حقوق", "income"),
    "other_income": ("Other income", "سایر درآمد، بازگشت وجه", "income"),
}
UNCAT = ("Uncategorized", "Nothing above fits or the text is not enough. نامشخص")

# expected Category per case: set of acceptable keys (None = internal, no Category)
EXP_CAT = {1: {"snacks"}, 2: {"loan_installment"}, 3: {"food"}, 4: {"education", "entertainment"},
           5: {"uncategorized", "shopping", "other", "food", "snacks", "groceries"},  # no note: anything plausible
           6: {"cash_withdrawal"}, 7: {"salary"}, 8: None, 9: {"transport"}, 10: {"fuel"}, 11: {"food"},
           12: {"entertainment"}, 13: {"health"}, 14: {"groceries", "food"}, 15: {"bills"}, 16: {"gifts"},
           17: {"snacks", "food"}, 18: {"other", "uncategorized"}}


def parse_samples(path):
    text = open(path, encoding="utf-8").read()
    cases = {}
    for m in re.finditer(r"=== (\d+) ===\n(.*?)(?=\n=== \d+ ===|\Z)", text, re.S):
        body, expect = [], None
        for ln in m.group(2).splitlines():
            if ln.startswith("expect:"):
                expect = ln[len("expect:"):].strip()
            else:
                body.append(ln)
        cases[int(m.group(1))] = ("\n".join(body).strip(), expect)
    return cases


def post(path, body, timeout=60):
    req = urllib.request.Request(BASE + path, json.dumps(body).encode(),
                                 {"Authorization": "Bearer " + KEY, "Content-Type": "application/json"})
    t = time.time()
    for attempt in (1, 2):  # retry once, as the contract says
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                return json.load(r), time.time() - t, attempt, None
        except urllib.error.HTTPError as e:
            err = f"HTTP {e.code}: {e.read().decode()[:200]}"
        except Exception as e:
            err = repr(e)
    return None, time.time() - t, attempt, err


def extract(inp):
    body = {"model": "gpt-4.1-mini", "temperature": 0,
            "response_format": {"type": "json_schema", "json_schema": SCHEMA},
            "messages": [{"role": "system", "content": SYSTEM}, {"role": "user", "content": inp}]}
    resp, lat, attempts, err = post("/openai/v1/chat/completions", body)
    if not resp:
        return None, lat, attempts, err, 0.0
    u = resp.get("usage", {})
    cost = u.get("prompt_tokens", 0) * P_IN + u.get("completion_tokens", 0) * P_OUT
    return json.loads(resp["choices"][0]["message"]["content"]), lat, attempts, None, cost


def to_transactions(ex):
    """Code side of the contract: unit conversion, internal-pair merge, note copy."""
    txs = []
    for e in ex["transactions"]:
        amt = e["amount"]
        if amt is not None and e["amount_unit"] == "rial":
            amt = amt // 10
        txs.append({"amount_toman": amt, "direction": e["direction"], "date": e["date"], "time": e["time"],
                    "confidence": e["confidence"], "note": ex["note"]})
    merged, used = [], set()
    for i, a in enumerate(txs):
        if i in used:
            continue
        for j in range(i + 1, len(txs)):
            b = txs[j]
            if j not in used and a["amount_toman"] is not None and a["amount_toman"] == b["amount_toman"] \
                    and {a["direction"], b["direction"]} == {"out", "in"} \
                    and (a["date"], a["time"]) == (b["date"], b["time"]):
                a = dict(a, direction="internal")
                used.add(j)
                break
        merged.append(a)
    return merged


def categorize(inp, direction):
    kind = "income" if direction == "in" else "expense"  # ambiguous -> expense default (#9)
    crit = {k: f"{n}. {h}" for k, (n, h, kd) in CATS.items() if kd == kind}
    crit["uncategorized"] = f"{UNCAT[0]}. {UNCAT[1]}"
    body = {"model": "jev-latest", "state": inp, "questions": {"category": {
        "type": "choice",
        "instructions": "Which category does this transaction belong to? The Owner's own note, if any, decides over the bank label.",
        "criteria": crit}}}
    resp, lat, attempts, err = post("/typesafe/v1/systemone", body)
    if not resp:
        return "uncategorized", None, None, lat, attempts, err, 0.0
    a = resp["answers"]["category"]
    choice, conf = a.get("choice"), a.get("confidence")
    final = choice if conf is not None and conf >= 0.7 else "uncategorized"
    cost = resp.get("usage", {}).get("input_tokens", resp.get("usage", {}).get("prompt_tokens", 0)) * JEV_IN
    return final, choice, conf, lat, attempts, None, cost


def expected(n, expect):
    """Parse 'amount | dir | cat | date' into comparable values (toman)."""
    parts = [p.strip() for p in expect.split("|")]
    amts = [int(x) // 10 for x in re.findall(r"\d{4,}", parts[0])] if "UNKNOWN" not in parts[0] else [None]
    d = parts[1].rstrip("?")
    date = re.match(r"\d{4}/\d{2}/\d{2}", parts[3]).group(0)
    return amts, d, date


def score(n, expect, txs, is_tx):
    amts, d, date = expected(n, expect)
    fails = []
    if not is_tx:
        fails.append("is_transaction=false")
    got_amts = [t["amount_toman"] for t in txs]
    if got_amts != amts:
        fails.append(f"amount {got_amts} != {amts}")
    dirs = {t["direction"] for t in txs}
    ok_dir = {d} if n != 13 else {"out", "ambiguous"}
    if not dirs or not dirs <= ok_dir:
        fails.append(f"direction {sorted(dirs)} != {d}")
    if any(t["date"] != date for t in txs):
        fails.append(f"date {[t['date'] for t in txs]} != {date}")
    return fails


def main():
    cases = parse_samples(SAMPLES)
    out, total = [], 0.0
    for run in range(RUNS):
        for n, (inp, expect) in cases.items():
            ex, lat, att, err, cost = extract(inp)
            rec = {"case": n, "run": run, "expect": expect, "ex_lat": round(lat, 2), "ex_attempts": att, "ex_err": err}
            total += cost
            if ex is None:
                rec["fails"] = ["extraction call failed"]
                out.append(rec); print(n, run, "FAIL", err); continue
            rec["raw"] = ex
            txs = to_transactions(ex)
            for t in txs:
                if t["direction"] == "internal":
                    t["category"] = None
                    continue
                final, choice, conf, jlat, jatt, jerr, jcost = categorize(inp, t["direction"])
                total += jcost
                t.update(category=final, jev_choice=choice, jev_conf=conf, jev_lat=round(jlat, 2), jev_err=jerr)
            rec["txs"] = txs
            fails = score(n, expect, txs, ex["is_transaction"])
            ok_c = EXP_CAT[n]
            for t in txs:
                if (ok_c is None) != (t["category"] is None) or (ok_c and t["category"] not in ok_c):
                    fails.append(f"category {t['category']} (jev {t.get('jev_choice')} {t.get('jev_conf')})")
            rec["note_ok"] = ex["note"]
            rec["fails"] = fails
            out.append(rec)
            print(n, run, "ok" if not fails else "; ".join(fails), "| note:", ex["note"], "|",
                  [(t["amount_toman"], t["direction"], t["date"], t["time"], t["category"], t.get("jev_conf")) for t in txs],
                  f"{lat:.1f}s", flush=True)
    json.dump(out, open("extraction_rerun_results.json", "w", encoding="utf-8"), ensure_ascii=False, indent=1)
    print(f"total cost ~${total:.5f}")


if __name__ == "__main__":
    main()
