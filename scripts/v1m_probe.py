"""Throwaway probe for v1m (tickets: "Get a v1m API key", "Test v1m's Persian category model").

Stdlib only. Safe on a bad connection: every call has a timeout and retries, and every
result is appended to a JSONL file as soon as it arrives, so a re-run resumes where it stopped.

Config (environment variables win over .env in the repo root):
  V1M_API_KEY   required
  V1M_MODEL     one or more model names, comma separated. Default: v1m-latest,rev-latest,jev-1.13.0
  V1M_URL       optional. Default https://v1m.ir/v1/systemone (full endpoint, not just the host)

Usage:
  python scripts/v1m_probe.py ping                 # one tiny call per model (ticket: Get a v1m API key)
  python scripts/v1m_probe.py category [--runs N]  # the 18 sample cases per model (ticket: Test v1m's category model)
  python scripts/v1m_probe.py all                  # ping, then category
  python scripts/v1m_probe.py summary              # rebuild summary.md from saved results, no network

Output goes to samples/v1m-results/ (gitignored, because it contains bank text):
  ping.jsonl, category.jsonl  one line per call: request model, case, raw response, headers, latency, error
  summary.md                  the short report to paste back into the conversation
"""
import json
import os
import re
import statistics
import sys
import time
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(ROOT, "samples", "v1m-results")
SAMPLES = os.path.join(ROOT, "samples", "forwarded-messages.txt")
DEFAULT_URL = "https://v1m.ir/v1/systemone"
DEFAULT_MODELS = "v1m-latest,rev-latest,jev-1.13.0"
TIMEOUT = 10
ATTEMPTS = 4

CATS = {  # key: english description, persian hint (same list as scripts/jev_sole_call.py)
    "food": ("Restaurants and meals", "غذا، ناهار، شام، رستوران"),
    "snacks": ("Snacks and treats", "تنقلات، خوراکی، شیرینی"),
    "groceries": ("Bread, supermarket and grocery shopping", "نان، سوپرمارکت، میوه، خرید خانه"),
    "transport": ("Taxi, metro, bus", "تاکسی، اسنپ، مترو، اتوبوس"),
    "fuel": ("Fuel for a car", "بنزین، سوخت"),
    "education": ("Courses, classes, books", "کلاس، دوره، کتاب، دانشگاه"),
    "entertainment": ("Cinema, subscriptions, hobbies, streaming", "سینما، نتفلیکس، اشتراک، تفریح"),
    "health": ("Pharmacy, doctor, medicine", "داروخانه، دکتر، دارو، درمان"),
    "bills": ("Electricity, water, gas, phone and internet bills", "قبض برق، آب، گاز، موبایل، اینترنت"),
    "loan_installment": ("Loan or bank installment payment", "قسط، وام"),
    "shopping": ("Clothes, electronics, online shopping", "لباس، دیجی‌کالا، خرید"),
    "cash_withdrawal": ("Cash taken out of an account", "برداشت نقدی"),
    "rent": ("Rent", "اجاره"),
    "gifts": ("Gifts for other people", "هدیه، کادو"),
    "other": ("Other expense", "سایر"),
    "salary": ("Salary income", "حقوق"),
    "other_income": ("Other income, refunds", "سایر درآمد، بازگشت وجه"),
    "uncategorized": ("Nothing above fits or the text is not enough", "نامشخص"),
}


def load_config():
    env = {}
    path = os.path.join(ROOT, ".env")
    if os.path.exists(path):
        for line in open(path, encoding="utf-8"):
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                k, v = line.split("=", 1)
                env[k.strip()] = v.strip().strip("\"'")
    get = lambda k, d=None: os.environ.get(k) or env.get(k) or d
    key = get("V1M_API_KEY")
    if not key:
        sys.exit("V1M_API_KEY is not set (put it in .env or the environment).")
    models = [m.strip() for m in get("V1M_MODEL", DEFAULT_MODELS).split(",") if m.strip()]
    return key, models, get("V1M_URL", DEFAULT_URL)


def call(url, key, body):
    """POST with retries on network errors, 429 and 5xx. Returns a record dict, never raises."""
    data = json.dumps(body, ensure_ascii=False).encode("utf-8")
    headers = {"Authorization": "Bearer " + key, "Content-Type": "application/json"}
    rec = {"status": None, "response": None, "headers": None, "error": None, "attempts": 0, "latency": None}
    for attempt in range(1, ATTEMPTS + 1):
        rec["attempts"] = attempt
        t = time.time()
        try:
            with urllib.request.urlopen(urllib.request.Request(url, data, headers), timeout=TIMEOUT) as r:
                rec.update(status=r.status, response=json.load(r), headers=dict(r.headers), error=None)
                rec["latency"] = round(time.time() - t, 3)
                return rec
        except urllib.error.HTTPError as e:
            raw = e.read().decode("utf-8", "replace")
            try:
                parsed = json.loads(raw)
            except ValueError:
                parsed = None
            rec.update(status=e.code, response=parsed, headers=dict(e.headers), error=f"HTTP {e.code}: {raw[:300]}")
            rec["latency"] = round(time.time() - t, 3)
            if e.code != 429 and e.code < 500:
                return rec  # 4xx other than 429 will not get better by retrying
        except Exception as e:  # DNS, timeout, reset, TLS: the bad-internet case
            rec.update(error=repr(e), latency=round(time.time() - t, 3))
        if attempt < ATTEMPTS:
            wait = 2 ** attempt
            print(f"    retry {attempt}/{ATTEMPTS - 1} in {wait}s: {rec['error'][:100]}")
            time.sleep(wait)
    return rec


def done_keys(path):
    """(model, case, run) of calls that already succeeded, so a re-run skips them."""
    keys = set()
    if os.path.exists(path):
        for line in open(path, encoding="utf-8"):
            r = json.loads(line)
            if r["status"] == 200:
                keys.add((r["model"], r["case"], r["run"]))
    return keys


def append(path, rec):
    with open(path, "a", encoding="utf-8") as f:
        f.write(json.dumps(rec, ensure_ascii=False) + "\n")


def parse_samples():
    text = open(SAMPLES, encoding="utf-8").read()
    cases = {}
    for m in re.finditer(r"=== (\d+) ===\n(.*?)(?=\n=== \d+ ===|\Z)", text, re.S):
        body, expect = [], None
        for ln in m.group(2).splitlines():
            if ln.startswith("expect:"):
                expect = ln[len("expect:"):].strip()
            else:
                body.append(ln)
        cases[int(m.group(1))] = ("\n".join(body).strip(), expect)  # expect never goes to the API
    return cases


def ping(url, key, models):
    os.makedirs(OUT, exist_ok=True)
    path = os.path.join(OUT, "ping.jsonl")
    for model in models:
        body = {"model": model, "state": "خرید از دیجی کالا",
                "questions": {"category": {"type": "choice", "instructions": "Which category fits?",
                                           "criteria": {"shopping": "Retail purchases", "food": "Meals and groceries"}}}}
        rec = {"model": model, "case": "ping", "run": 0, "url": url, **call(url, key, body)}
        append(path, rec)
        shape = list((rec["response"] or {}).get("answers", {}).get("category", {}).keys())
        print(f"ping {model}: status={rec['status']} latency={rec['latency']}s answer_fields={shape} {rec['error'] or ''}")


def category(url, key, models, runs):
    os.makedirs(OUT, exist_ok=True)
    path = os.path.join(OUT, "category.jsonl")
    done = done_keys(path)
    cases = parse_samples()
    criteria = {k: f"{d}. {p}" for k, (d, p) in CATS.items()}
    instr = ("Which category does this transaction belong to? The Owner's own note, if any, "
             "decides over the bank label.")
    for model in models:
        for run in range(runs):
            for n, (state, expect) in cases.items():
                if (model, n, run) in done:
                    continue
                body = {"model": model, "state": state,
                        "questions": {"category": {"type": "choice", "instructions": instr, "criteria": criteria}}}
                rec = {"model": model, "case": n, "run": run, "expect": expect, "url": url, **call(url, key, body)}
                append(path, rec)
                ans = ((rec["response"] or {}).get("answers") or {}).get("category") or {}
                print(f"{model} case {n:02d} run {run}: {ans.get('choice')} conf={ans.get('confidence')} "
                      f"{rec['latency']}s {rec['error'] or ''}")


def load(path):
    return [json.loads(l) for l in open(path, encoding="utf-8")] if os.path.exists(path) else []


def pct(vals, q):
    vals = sorted(vals)
    return vals[min(len(vals) - 1, int(q * len(vals)))] if vals else None


def summary():
    lines = ["# v1m probe summary", "", f"Generated {time.strftime('%Y-%m-%d %H:%M')}. "
             "Paste this back into the conversation; raw data is in the .jsonl files next to it.", ""]
    ping_rows = load(os.path.join(OUT, "ping.jsonl"))
    if ping_rows:
        lines += ["## Ping (ticket: Get a v1m API key)", "", "| model | status | latency s | answer fields | error |", "|---|---|---|---|---|"]
        for r in ping_rows:
            ans = ((r["response"] or {}).get("answers") or {}).get("category") or {}
            lines.append(f"| {r['model']} | {r['status']} | {r['latency']} | {', '.join(ans) or '-'} | {(r['error'] or '')[:80]} |")
        ok = next((r for r in ping_rows if r["status"] == 200), None)
        if ok:
            lines += ["", "Interesting response headers (rate limit, cost, credit):", ""]
            for h, v in ok["headers"].items():
                if re.search(r"limit|remain|cost|credit|price|balance|usage|request-id", h, re.I):
                    lines.append(f"- `{h}`: {v}")
            lines += ["", "One full response:", "", "```json", json.dumps(ok["response"], ensure_ascii=False, indent=1), "```"]
        lines.append("")
    rows = load(os.path.join(OUT, "category.jsonl"))
    if rows:
        lines += ["## Category (ticket: Test v1m's Persian category model)", ""]
        for model in dict.fromkeys(r["model"] for r in rows):
            mr = [r for r in rows if r["model"] == model]
            good = [r for r in mr if r["status"] == 200]
            lat = [r["latency"] for r in good]
            toks = [r["response"]["usage"]["input_tokens"] for r in good if (r["response"] or {}).get("usage", {}).get("input_tokens")]
            lines += [f"### {model}", "",
                      f"- calls: {len(mr)}, ok: {len(good)}, failed: {len(mr) - len(good)}",
                      f"- latency p50 / p95: {pct(lat, .5)} / {pct(lat, .95)} s" if lat else "- latency: n/a",
                      f"- mean input tokens: {round(statistics.mean(toks))}" if toks else "- input tokens: not reported",
                      "", "| case | run | chose | confidence | expected (Owner's draft) |", "|---|---|---|---|---|"]
            for r in good:
                a = r["response"]["answers"]["category"]
                lines.append(f"| {r['case']} | {r['run']} | {a.get('choice')} | {a.get('confidence')} | {r['expect']} |")
            fields = sorted({k for r in good for k in r["response"]["answers"]["category"]})
            lines += ["", f"Answer fields seen: {fields}", ""]
            errs = [r for r in mr if r["status"] != 200]
            for r in errs[:5]:
                lines.append(f"- case {r['case']} run {r['run']} failed: {r['error']}")
            lines.append("")
    if not ping_rows and not rows:
        lines.append("No results yet. Run `ping` or `category` first.")
    path = os.path.join(OUT, "summary.md")
    os.makedirs(OUT, exist_ok=True)
    open(path, "w", encoding="utf-8").write("\n".join(lines) + "\n")
    print(f"wrote {path}")


def main():
    args = sys.argv[1:]
    cmd = args[0] if args else "all"
    runs = int(args[args.index("--runs") + 1]) if "--runs" in args else 1
    if cmd == "summary":
        return summary()
    if cmd not in ("ping", "category", "all"):
        sys.exit(__doc__)
    key, models, url = load_config()
    print(f"endpoint {url}\nmodels   {', '.join(models)}\nkey      {key[:9]}...{key[-3:]} (masked)")
    if cmd in ("ping", "all"):
        ping(url, key, models)
    if cmd in ("category", "all"):
        category(url, key, models, runs)
    summary()


if __name__ == "__main__":
    main()
