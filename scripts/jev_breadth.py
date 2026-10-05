"""Throwaway: how broad an Input can Jev (Metis /typesafe/v1/systemone) handle alone? Ticket #21.
Extends scripts/jev_sole_call.py: code builds options from number tokens, Jev only chooses.
New in this probe: multiplier merge («۴۵۰ هزار» -> one option 450000), relative-date options,
a "is this a transaction" gate and an item-count question.
Usage: python scripts/jev_breadth.py [runs]   (reads LLM_API_KEY from .env)
Cases marked REAL come from the gitignored samples/; DRAFT cases were written by the agent.
"""
import json, re, sys, time, urllib.request, urllib.error

URL = "https://api.metisai.ir/typesafe/v1/systemone"
RUNS = int(sys.argv[1]) if len(sys.argv) > 1 else 2
KEY = None
for line in open(".env", encoding="utf-8"):
    if line.startswith("LLM_API_KEY="):
        KEY = line.split("=", 1)[1].strip()

TR = str.maketrans("۰۱۲۳۴۵۶۷۸۹٠١٢٣٤٥٦٧٨٩٫٬", "01234567890123456789.,")
MULT = {"هزار": 1000, "میلیون": 10**6, "ملیون": 10**6, "میلیارد": 10**9}

CATS = {
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
    "other_income": ("Other income, refunds, money received back", "سایر درآمد، بازگشت وجه"),
    "uncategorized": ("Nothing above fits or the text is not enough", "نامشخص"),
}

DATE_RE = re.compile(r"\d{4}/\d{2}/\d{2}|\d{4}-\d{2}:\d{2}|\d{2}:\d{2}|\d{2}/\d{2}/\d{2}")
NUM_RE = re.compile(r"\d[\d,]*(?:\.\d+)?")
MULT_RE = re.compile(r"(\d[\d,]*(?:\.\d+)?)\s*(هزار|میلیون|ملیون|میلیارد)")


def extract(inp):
    norm = inp.translate(TR)
    dates, nums = {}, {}
    for ln in norm.splitlines():
        for d in DATE_RE.findall(ln):
            dates.setdefault(d, ln.strip())
        rest = DATE_RE.sub(" ", ln)
        # multiplier merge: "450 هزار" -> 450000
        for m in MULT_RE.finditer(rest):
            v = int(round(float(m.group(1).replace(",", "")) * MULT[m.group(2)]))
            nums.setdefault(str(v), ln.strip())
        rest = MULT_RE.sub(" ", rest)
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
                       "instructions": "Did money leave the owner (out), arrive (in), or is it unclear (ambiguous)? A minus sign after the amount means out, a plus sign means in.",
                       "criteria": {"out": "Money left the owner: purchase, withdrawal, payment, minus sign, money lent",
                                    "in": "Money arrived: deposit, salary, plus sign, money received back",
                                    "ambiguous": "Cannot tell"}}
    qs["category"] = {"type": "choice",
                      "instructions": "Which category does this transaction belong to? The Owner's own note, if any, decides over the bank label.",
                      "criteria": {k: f"{d}. {p}" for k, (d, p) in CATS.items()}}
    dq = {f"d_{d}": f"The date or time {d}, in the line: {ln}" for d, ln in dates.items() if "/" in d or "-" in d}
    dq["today"] = "The text says today (امروز) or gives no date but describes something happening now"
    dq["yesterday"] = "The text says yesterday (دیروز)"
    dq["day_before"] = "The text says the day before yesterday (پریروز)"
    dq["written"] = "A date written with a month name, a weekday or a phrase such as last week"
    dq["none"] = "The text has no date at all"
    qs["date"] = {"type": "choice", "instructions": "Which of these is the date the transaction happened?", "criteria": dq}
    qs["tx"] = {"type": "choice",
                "instructions": "Does the text report money that was actually spent, received or moved? Either a bank notice or the Owner's own note about something they paid or received counts.",
                "criteria": {"yes": "A payment, purchase, withdrawal, deposit or transfer that already happened, from a bank notice or from the Owner's own note",
                             "no": "An advertisement, discount code, reminder, future or unpaid bill, balance-only notice, one-time password, greeting or chat"}}
    qs["count"] = {"type": "choice",
                   "instructions": "How many separate transactions does the text describe? Count each bank notice and each separately priced item in a note.",
                   "criteria": {"zero": "No transaction", "one": "Exactly one transaction",
                                "two": "Exactly two transactions", "three_plus": "Three or more transactions"}}
    return {"model": "jev-latest", "state": norm, "questions": qs}


def call(body, tries=5):
    """Retry 5xx with 4 s backoff; the first run hit a Metis outage (503) on 88 of 90 calls."""
    for i in range(tries):
        r = call_once(body)
        if r[2] is None or "HTTP 5" not in r[2]:
            return r[0], r[1], r[2], i + 1
        time.sleep(4)
    return r[0], r[1], r[2], tries


def call_once(body):
    req = urllib.request.Request(URL, json.dumps(body).encode(), {"Authorization": "Bearer " + KEY, "Content-Type": "application/json"})
    t = time.time()
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return json.load(r), time.time() - t, None
    except urllib.error.HTTPError as e:
        return None, time.time() - t, f"HTTP {e.code}: {e.read().decode()[:300]}"
    except Exception as e:
        return None, time.time() - t, repr(e)


REAL = json.load(open('samples/jev-breadth-real.json', encoding='utf-8'))  # gitignored: real bank text stays out of the repo

BM = REAL["BM"]  # a Melli Bale-bot message, see samples/jev-breadth-real.json

# dim, id, source, input, expected. Expected keys: amt (printed/merged value or "none"), dir, cat (list), date, tx, count.
# Missing key = not scored (shown for reading only).
CASES = [
    # A. note-only
    ("note-only", "a1", "DRAFT", "ناهار ۳۵۰ تومن", dict(amt="350", dir="out", cat=["food"], tx="yes", count="one")),
    ("note-only", "a2", "DRAFT", "حقوق گرفتم ۵۰ میلیون", dict(amt="50000000", dir="in", cat=["salary"], tx="yes", count="one")),
    ("note-only", "a3", "DRAFT", "تاکسی ۸۰ هزار تومن", dict(amt="80000", dir="out", cat=["transport"], tx="yes", count="one")),
    ("note-only", "a4", "DRAFT", "علی ۲۰۰ هزار تومن طلبشو پس داد", dict(amt="200000", dir="in", cat=["other_income"], tx="yes", count="one")),
    ("note-only", "a5", "DRAFT", "امروز خرید کردم", dict(amt="none", tx="yes")),
    # B. number words and multipliers
    ("words", "b1", "DRAFT", "۴۵۰ هزار تومن شام", dict(amt="450000", dir="out", cat=["food"], tx="yes", count="one")),
    ("words", "b2", "DRAFT", "دو میلیون و دویست هزار تومن اجاره", dict(amt="none-expected-fail", dir="out", cat=["rent"], tx="yes", count="one")),
    ("words", "b3", "DRAFT", "سه تا بلیط سینما ۱۵ هزار تومنی", dict(amt="none-expected-fail", dir="out", cat=["entertainment"], tx="yes", count="one")),
    ("words", "b4", "DRAFT", "۱.۵ میلیون تومن لباس", dict(amt="1500000", dir="out", cat=["shopping"], tx="yes", count="one")),
    ("words", "b5", "DRAFT", "پنجاه هزار تومن بنزین زدم", dict(amt="none-expected-fail", dir="out", cat=["fuel"], tx="yes", count="one")),
    # C. several items
    ("multi-item", "c1", "DRAFT", "سینما ۴۵۰ هزار، پاپکورن ۸۰ هزار", dict(amt="multi", count="two")),
    ("multi-item", "c2", "DRAFT", "امروز نون ۳۰ هزار میوه ۱۲۰ هزار گوشت ۵۰۰ هزار تومن", dict(amt="multi", count="three_plus")),
    ("multi-item", "c3", "DRAFT", "۲۰۰ هزار تومن بنزین و ۱۰۰ هزار تومن تاکسی", dict(amt="multi", count="two")),
    ("multi-item", "c4", "DRAFT", "خرید از سوپرمارکت ۳۴۰ هزار تومن شد که ۶۰ هزارش میوه بود", dict(amt="340000", cat=["groceries"], count="one")),
    # D. dates
    ("date", "d1", "DRAFT", "دیروز ۲۵۰ تومن نون", dict(amt="250", date="yesterday", cat=["groceries", "food"], tx="yes")),
    ("date", "d2", "DRAFT", "پریروز ۱۸۰ هزار تومن دکتر", dict(amt="180000", date="day_before", cat=["health"], tx="yes")),
    ("date", "d3", "DRAFT", "۱۴۰۵/۰۷/۰۲ ۴۰۰ هزار تومن کرایه خونه", dict(amt="400000", date="d_1405/07/02", cat=["rent"], tx="yes")),
    ("date", "d4", "DRAFT", "هفته پیش ۹۰ هزار تومن کتاب", dict(amt="90000", date="written", cat=["education"], tx="yes")),
    ("date", "d5", "DRAFT", "۵ مهر ۲۰۰ هزار تومن داروخانه", dict(amt="200000", date="written", cat=["health"], tx="yes")),
    ("date", "d6", "DRAFT", "۱۸۰ هزار تومن تاکسی", dict(amt="180000", date="today", cat=["transport"], tx="yes")),
    # E. other banks / formats (REAL = from samples/, DRAFT = invented from public descriptions of formats)
    ("other-banks", "e1", "REAL", REAL["e1"], dict(amt="4509614", dir="out", cat=["shopping"], date="d_1405/07/11", tx="yes", count="one")),
    ("other-banks", "e2", "REAL", REAL["e2"], dict(amt="68000000", dir="in", date="d_1405/06/28", tx="yes", count="one")),
    ("other-banks", "e3", "REAL", REAL["e3"], dict(amt="100000000", dir="in", date="d_0709-22:25", tx="yes", count="one")),
    ("other-banks", "e4", "DRAFT", "بانک ملت\nبرداشت از حساب\nمبلغ: 1,200,000 ریال\nمانده: 8,400,000 ریال\n1405/07/10 - 12:05\nشام", dict(amt="1200000", dir="out", cat=["food"], date="d_1405/07/10", tx="yes", count="one")),
    ("other-banks", "e5", "DRAFT", "بانک سامان\nخرید\n۲۴۰,۰۰۰- ریال\n۰۷/۱۰ ۱۴:۳۰\nمانده ۵,۱۰۰,۰۰۰\nاسنپ", dict(amt="240000", dir="out", cat=["transport"], tx="yes", count="one")),
    ("other-banks", "e6", "DRAFT", "بانک سپه\nواریز: 12,000,000 ریال\nبابت: حقوق\nمانده: 14,300,000\n1405/07/01", dict(amt="12000000", dir="in", cat=["salary"], date="d_1405/07/01", tx="yes", count="one")),
    ("other-banks", "e7", "DRAFT", "تراکنش موفق\nپرداخت قبض\nمبلغ: ۸۵,۰۰۰ تومان\nتاریخ: ۱۴۰۵/۰۷/۰۹\nمانده: ۲,۴۰۰,۰۰۰ تومان", dict(amt="85000", dir="out", cat=["bills"], date="d_1405/07/09", tx="yes", count="one")),
    ("other-banks", "e8", "DRAFT", "کارت **1234\nCr 5,000,000 IRR\nBal 20,000,000\n07/10", dict(amt="5000000", dir="in", tx="yes", count="one")),
    # F. two to five bank messages
    ("multi-bank", "f1", "REAL", REAL["f1"], dict(amt="multi", dir="out", count="three_plus")),
    ("multi-bank", "f2", "REAL", REAL["f2"], dict(amt="multi", count="two")),
    ("multi-bank", "f3", "REAL", REAL["f3"], dict(amt="multi", count="three_plus")),
    # G. noise
    ("noise", "g1", "DRAFT", "کد تخفیف ۵۰٪ دیجی‌کالا تا سقف ۲۰۰,۰۰۰ تومان فقط تا امشب! لغو۱۱", dict(tx="no", count="zero")),
    ("noise", "g2", "DRAFT", "قبض برق شما به مبلغ ۴۸۰,۰۰۰ ریال صادر شد. مهلت پرداخت ۱۴۰۵/۰۷/۲۵", dict(tx="no", count="zero")),
    ("noise", "g3", "DRAFT", "بانك ملي ايران\nمانده حساب: 16,500,000 ریال\n1405/07/10", dict(tx="no", count="zero")),
    ("noise", "g4", "DRAFT", "رمز یکبار مصرف شما: ۴۸۲۹۱۷\nاین رمز را به کسی ندهید", dict(tx="no", count="zero")),
    ("noise", "g5", "DRAFT", "سلام خوبی؟ فردا میای؟", dict(tx="no", count="zero")),
    ("noise", "g6", "DRAFT", "یادآوری: قسط وام شما به مبلغ ۲۰,۰۵۱,۰۶۱ ریال در تاریخ ۱۴۰۵/۰۷/۲۰ سررسید می‌شود", dict(tx="no", count="zero")),
    ("noise", "g7", "DRAFT", "فردا باید ۵۰۰ هزار تومن بدم به علی", dict(tx="no", count="zero")),
    # H. long, mixed, typos, digits
    ("robust", "h1", "DRAFT", BM + "\nامروز با بچه‌ها رفتیم بیرون اول رفتیم کافه بعد یه کم قدم زدیم بعد هم رفتیم سینما بعد از اون یه چیزی خوردیم و خلاصه خیلی خوش گذشت ولی پول زیادی خرج شد دیگه از این کارا نمیکنم", dict(amt="320000", dir="out", date="d_1405/07/06", tx="yes", count="one")),
    ("robust", "h2", "DRAFT", "نهر ۳۵۰ تمن", dict(amt="350", dir="out", cat=["food"], tx="yes", count="one")),
    ("robust", "h3", "DRAFT", "Snapp taxi 120 هزار تومن", dict(amt="120000", dir="out", cat=["transport"], tx="yes", count="one")),
    ("robust", "h4", "DRAFT", "٣٥٠ تومن ناهار", dict(amt="350", dir="out", cat=["food"], tx="yes", count="one")),
    ("robust", "h5", "REAL", REAL["h5"], dict(amt="320000", dir="out", cat=["entertainment"], tx="yes", count="one")),
]


def main():
    out = []
    for run in range(RUNS):
        for dim, cid, src, inp, exp in CASES:
            resp, lat, err, tries = call(build(inp))
            rec = {"dim": dim, "case": cid, "src": src, "run": run, "exp": exp, "lat": round(lat, 2), "err": err, "tries": tries}
            if resp:
                a = resp["answers"]
                rec["usage"] = resp.get("usage")
                for q in ("amount", "direction", "category", "date", "tx", "count"):
                    rec[q] = a[q].get("choice")
                    rec[q + "_conf"] = a[q].get("confidence")
            out.append(rec)
            print(cid, run, rec.get("amount"), rec.get("amount_conf"), rec.get("direction"), rec.get("category"), rec.get("date"), rec.get("tx"), rec.get("tx_conf"), rec.get("count"), rec["lat"], err or "", flush=True)
    json.dump(out, open("scripts/jev_breadth_results.json", "w", encoding="utf-8"), ensure_ascii=False, indent=1)


if __name__ == "__main__":
    main()
