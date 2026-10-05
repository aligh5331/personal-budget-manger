# v1m category model test

Ticket "Test v1m's Persian category model" (map #1), with "Get a v1m API key". Run 2026-10-06 by the Owner with `scripts/v1m_probe.py`; raw results stay in the gitignored `samples/v1m-results/`.

## Verdict

v1m is not usable for the Category step and should not receive bank text. Its output does not depend on the model name, is mostly "uncategorized", and the service is unreliable. Use Jev through Metis (`docs/research/jev-sole-call.md`).

## Setup

- Endpoint `https://v1m.ir/v1/systemone`, request shape `instructions` + `criteria` (TypeSafe shape).
- Models tried: `v1m-latest`, `rev-latest`, `jev-1.13.0`.
- Ping: one tiny Choice call per model. Category: the 18 cases from `samples/forwarded-messages.txt` (note + bank text as `state`), the 18-key Category list from `scripts/jev_sole_call.py` as `criteria`, one run per model.

## Findings

- **Key and ping.** The key authenticates. `v1m-latest` timed out on the ping; `rev-latest` (3.4 s) and `jev-1.13.0` (0.14 s) returned 200.
- **Response shape.** `answers.<q>.{type, choice, confidence, probabilities}`. It is `probabilities`, as TypeSafe documents, not v1m's documented `distribution`. Resolves the doc mismatch in `jev-v1m.md`.
- **Model name is ignored.** On the 17 cases that more than one model answered, all three models returned identical answers, probabilities equal to the last decimal. The response echoes the requested model name, but every response carries `x-engine-tier: v1m-systemone-fastpath` and `x-process-time-ms` of 9 to 64 ms. This looks like one cheap engine behind all names, not Jev. (Model names `v1m-latest`, `rev-latest`, `jev-1.13.0` all work as strings; none behaves differently.)
- **Accuracy.** 16 of 17 answered cases came back `uncategorized` (the last option in the list) at 0.31 to 0.52 confidence, including clear ones: lunch (case 3), taxi with a typo (case 9), salary (case 7). The only other answer was case 15: `food` at 0.9, every other option at 0.006, expected `utilities`. So roughly 0 of 17 useful. Jev on Metis gave acceptable Categories on 17 of 17 on the same cases and list (`jev-sole-call.md`).
- **Reliability.** `v1m-latest`: 3 timeouts of 18 calls. `rev-latest`: 11 of 18 calls returned 502 (nginx). `jev-1.13.0`: 1 timeout of 18.
- **Latency.** Successful calls took about 0.13 to 0.9 s, fast because the engine is shallow.
- **Input tokens** reported: about 1,000 to 1,200 per call.
- **Price per call.** Not measurable. Responses carry no cost or credit header, and v1m publishes no price list.

## Not checked

- Dashboard plan, credit and limits (the Owner reads these in the web UI).
- Whether v1m's Persian-tuned models (`qwen2.5-1.5b-systemone`, `laya-multilingual-onnx` from `jev-v1m.md`) behave differently; they were not tried by name.
- Whether any money was taken.

## Answer

Not more accurate, not cheaper in any provable way, not reliable, and a third party that would see bank text. Drop v1m. Both v1m tickets close with this finding.
