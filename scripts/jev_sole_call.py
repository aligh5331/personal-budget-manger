"""Throwaway: can Jev (Metis /typesafe/v1/systemone) be the only LLM call? Ticket #17.
Code pulls number-like tokens out of the Input; Jev only chooses among them.
Usage: python scripts/jev_sole_call.py [runs]   (reads LLM_API_KEY from .env)
"""
import json, re, sys, time, urllib.request, urllib.error

URL = "https://api.metisai.ir/typesafe/v1/systemone"
PRICE_IN = 0.046 / 1e6  # USD per input token, Metis pricing, output is free
RUNS = int(sys.argv[1]) if len(sys.argv) > 1 else 2

KEY = None
for line in open(".env", encoding="utf-8"):
    if line.startswith("LLM_API_KEY="):
        KEY = line.split("=", 1)[1].strip()

TR = str.maketrans("۰۱۲۳۴۵۶۷۸۹٠١٢٣٤٥٦٧٨٩", "01234567890123456789")

CATS = {  # key: (english description, persian hint, kind)
    "food": ("Restaurants and meals", "غذا، ناهار، شام، رستوران", "out"),
    "snacks": ("Snacks and treats", "تنقلات، خوراکی، شیرینی", "out"),
    "groceries": ("Bread, supermarket and grocery shopping", "نان، سوپرمارکت، میوه، خرید خانه", "out"),
    "transport": ("Taxi, metro, bus", "تاکسی، اسنپ، مترو، اتوبوس", "out"),
    "fuel": ("Fuel for a car", "بنزین، سوخت", "out"),
    "education": ("Courses, classes, books", "کلاس، دوره، کتاب، دانشگاه", "out"),
    "entertainment": ("Cinema, subscriptions, hobbies, streaming", "سینما، نتفلیکس، اشتراک، تفریح", "out"),
    "health": ("Pharmacy, doctor, medicine", "داروخانه، دکتر، دارو، درمان", "out"),
    "bills": ("Electricity, water, gas, phone and internet bills", "قبض برق، آب، گاز، موبایل، اینترنت", "out"),
    "loan_installment": ("Loan or bank installment payment", "قسط، وام", "out"),
    "shopping": ("Clothes, electronics, online shopping", "لباس، دیجی‌کالا، خرید", "out"),
    "cash_withdrawal": ("Cash taken out of an account", "برداشت نقدی", "out"),
    "rent": ("Rent", "اجاره", "out"),
    "gifts": ("Gifts for other people", "هدیه، کادو", "out"),
    "other": ("Other expense", "سایر", "out"),
    "salary": ("Salary income", "حقوق", "in"),
    "other_income": ("Other income, refunds", "سایر درآمد، بازگشت وجه", "in"),
    "uncategorized": ("Nothing above fits or the text is not enough", "نامشخص", "out"),
}

DATE_RE = re.compile(r"\d{4}/\d{2}/\d{2}|\d{4}-\d{2}:\d{2}|\d{2}:\d{2}")
NUM_RE = re.compile(r"\d[\d,]*")


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


def extract(inp):
    norm = inp.translate(TR)
    lines = norm.splitlines()
    dates, nums = {}, {}
    for ln in lines:
        for d in DATE_RE.findall(ln):
            dates.setdefault(d, ln.strip())
        rest = DATE_RE.sub(" ", ln)
        for n in NUM_RE.findall(rest):
            v = n.replace(",", "")
            if v:
                nums.setdefault(v, ln.strip())
    return norm, nums, dates


def build(inp):
    norm, nums, dates = extract(inp)
    qs = {}
    amt = {f"n_{v}": f"The number {v}, found in the line: {ln}" for v, ln in nums.items()}
    amt["none"] = "No listed number is the amount of money moved"
    qs["amount"] = {"type": "choice",
                    "instructions": "Which number is the amount of money that this transaction moved? Never the account balance.",
                    "criteria": amt}
    qs["direction"] = {"type": "choice",
                       "instructions": "Did money leave the account (out), arrive (in), or is it unclear (ambiguous)? A minus sign after the amount means out, a plus sign means in.",
                       "criteria": {"out": "Money left the account: purchase, withdrawal, minus sign",
                                    "in": "Money arrived: deposit, salary, plus sign",
                                    "ambiguous": "Cannot tell"}}
    qs["category"] = {"type": "choice",
                      "instructions": "Which category does this transaction belong to? The Owner's own note, if any, decides over the bank label.",
                      "criteria": {k: f"{d}. {p}" for k, (d, p, _) in CATS.items()}}
    dq = {f"d_{d}": f"The date or time {d}, in the line: {ln}" for d, ln in dates.items() if "/" in d or "-" in d}
    dq["none"] = "The text has no date"
    qs["date"] = {"type": "choice", "instructions": "Which of these is the date the transaction happened?", "criteria": dq}
    return {"model": "jev-latest", "state": norm, "questions": qs}


def call(body):
    req = urllib.request.Request(URL, json.dumps(body).encode(), {"Authorization": "Bearer " + KEY, "Content-Type": "application/json"})
    t = time.time()
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return json.load(r), time.time() - t, None
    except urllib.error.HTTPError as e:
        return None, time.time() - t, f"HTTP {e.code}: {e.read().decode()[:300]}"
    except Exception as e:
        return None, time.time() - t, repr(e)


def main():
    cases = parse_samples("samples/forwarded-messages.txt")
    out = []
    for run in range(RUNS):
        for n, (inp, expect) in cases.items():
            body = build(inp)
            resp, lat, err = call(body)
            rec = {"case": n, "run": run, "expect": expect, "lat": round(lat, 2), "err": err}
            if resp:
                a = resp["answers"]
                rec["usage"] = resp.get("usage")
                for q in ("amount", "direction", "category", "date"):
                    rec[q] = a[q].get("choice")
                    rec[q + "_conf"] = a[q].get("confidence")
                    rec[q + "_probs"] = a[q].get("probabilities") or a[q].get("distribution")
                if n == 1 and run == 0:
                    rec["raw_keys"] = {k: list(v.keys()) for k, v in a.items()}
            out.append(rec)
            print(n, run, rec.get("amount"), rec.get("direction"), rec.get("category"), rec.get("date"), rec["lat"], err or "")
    json.dump(out, open("scripts/jev_sole_call_results.json", "w", encoding="utf-8"), ensure_ascii=False, indent=1)


if __name__ == "__main__":
    main()
