# Local open-source Jev-like Category classifiers (CPU-only)

Research for ticket #20 (map #1). Researched 2026-10-06 from model cards, papers and repo READMEs only. No model was run. Scope: the Category step (short Persian text plus a fixed Category list with descriptions, pick one with a usable confidence) on the home lab (`ali@lab`, no usable GPU).

## Answer

- **No open Jev exists.** TypeSafe's pages and docs state nothing about weights, size, base model, licence or self-hosting. "The same model weights serve every customer account", and only an API price is given. Treat Jev as closed.
- **No candidate is shown to match Jev for Persian.** Nothing first-party measures Persian Category choice for any model below. Jev scored 17/17 acceptable Categories on 18 cases at about $0.00006 per Input (`docs/research/jev-sole-call.md`), about $0.01 a month. A local model saves that and costs a service to run, a model to calibrate and a labelled data set to build.
- **Best local shortlist, if the Owner wants one anyway** (ranked):
  1. **Embeddings plus nearest label, with a small head trained on the Owner's own labelled Inputs** (`intfloat/multilingual-e5-small`, 118M params, MIT).
  2. **Multilingual NLI zero-shot** (`mDeBERTa-v3-base-xnli-multilingual-nli-2mil7`, 279M params, MIT).
  3. **Small instruct LLM, 0.6B to 1.7B quantised, via `llama-server`** (Qwen3 or Qwen2.5, Apache 2.0).
- **Recommendation:** keep Jev on Metis as the default. Run a prototype ticket only if there is a reason beyond cost (privacy of bank text, Metis availability). The "one that runs without a GPU" the Owner heard of is not identifiable from primary sources; see "The one the Owner heard of".

## Jev

- "Jev is TypeSafe's first public System One Model", trained with "Reinforcement Learning for Calibrated Decisions (RLCD)", $42 per billion input tokens. Pages list no architecture, size, base model, open weights, licence or on-prem option. (https://typesafe.ai, https://docs.typesafe.ai/models.md)
- The docs also say Jev is not fine-tuned on customer data. So there is no export of a tuned Jev either.
- v1m (researched in `jev-v1m.md`) advertises its own small models, `qwen2.5-1.5b-systemone` and `laya-multilingual-onnx`, as Persian-tuned. Those are v1m's, hosted, unmeasured, and not published as weights in anything checked here.

## Candidates

CPU latency and RAM: **no primary source publishes CPU numbers for these models**. Only the mDeBERTa card gives speed (about 1,100 to 1,900 texts/s, on an A100 GPU), which says nothing about the lab. The sizes below are what a prototype must turn into measured latency and RAM.

### 1. Embeddings plus nearest label (or small head)

How it works: embed the Input once, embed each Category's label plus description once (cache them), pick the highest cosine similarity. Or train a logistic regression on embeddings of the Owner's own labelled Inputs. SetFit (https://github.com/huggingface/setfit, Apache 2.0 licence file) fine-tunes a sentence-transformer on a few labelled examples per class; its README claims 8 examples per class matched RoBERTa-Large fully fine-tuned on one sentiment dataset. Training would be done once in Python, offline. The README does not mention ONNX export.

| Model | Params | Licence | Persian evidence | Export |
|---|---|---|---|---|
| `intfloat/multilingual-e5-small` | 117.65M (12 layers, 384-dim) | MIT | `fa` in the card's language list; card says low-resource languages "may see performance degradation"; MTEB Massive intent (fa) accuracy 65.5, scenario 67.2 on the card's table (a close cousin task, different from Category choice) | Official repo ships `onnx/model.onnx`, `model_O4.onnx`, `model_qint8_avx512_vnni.onnx` plus tokenizer files |
| `intfloat/multilingual-e5-base` | 278M | MIT | FaMTEB average 57.03, classification 59.97 (https://arxiv.org/abs/2502.11571) | Xenova ONNX conversions exist (not first-party) |
| `BAAI/bge-m3` | about 568M (1024-dim, xlm-roberta based) | MIT | FaMTEB average 59.10, classification 61.74 | `onnx/model.onnx` in the official repo |
| `Qwen/Qwen3-Embedding-0.6B` | 0.6B, 28 layers | Apache 2.0 | "100+ languages"; no Persian-specific number found | third-party ONNX only (`onnx-community`) |
| `sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2` | 117.65M, 384-dim | Apache 2.0 | `fa` in card's language list; 128-token limit | many ONNX variants (int8) in repo |
| `PartAI/Tooka-SBERT` (Persian only) | 353M, 1024-dim | Apache 2.0 | Persian-trained; no FaMTEB number checked | safetensors only, no ONNX in repo |

Notes:
- FaMTEB (63 Persian datasets, 15 models) puts bge-m3 slightly ahead of e5-base and e5-large; e5-small is not in the table I could read. The Hakim paper (https://arxiv.org/abs/2505.08435) reports its own, differently scaled averages (e5-large 64.40, bge-m3 65.29; Hakim 124M 73.81; Hakim-small 38M 70.45), so numbers from the two papers cannot be mixed. I found no public weights for Hakim on Hugging Face (`PartAI/Hakim` returned "invalid username or password", i.e. not public under that id). Not verified further.
- The e5 card says cosine scores "distribute around 0.7 to 1.0" because of a low training temperature and that "what matters is the relative order", so an absolute cosine threshold is not a confidence. Use the margin between top-1 and top-2, or softmax over scaled similarities, and calibrate on labelled data.
- e5 needs a `query: ` prefix on every input ("otherwise you will see a performance degradation"); input is truncated at 512 tokens.
- Confidence into the Flagged transaction rule: with a trained head, `predict_proba` is a probability and the existing gate (below 0.7 asks or flags) applies directly after calibration. With nearest-label only, margin is a heuristic that needs a threshold chosen on held-out cases.
- Weakness: embedding similarity ranks topical closeness. It cannot reason about "the note wins over the bank's label", and a bank line with a merchant name plus a short Persian note may pull toward the merchant. A trained head fixes some of this, and needs labelled data (the 18 cases are not enough to train, only to smoke-test).
- Go: `hugot` (https://github.com/knights-analytics/hugot, Apache 2.0) runs ONNX feature-extraction pipelines from Go with no Python; its default pure-Go backend needs no cgo, the ONNX Runtime backend (`-tags ORT`) is faster and needs cgo. `onnxruntime_go` (MIT) is the low-level option: cgo, a matching ONNX Runtime shared library (it pins one version) and no tokenizer, so tokenisation is on you. A Python sidecar (sentence-transformers) is the zero-friction alternative.

### 2. Multilingual NLI zero-shot

`MoritzLaurer/mDeBERTa-v3-base-xnli-multilingual-nli-2mil7`: 278.8M params, MIT, built for "multilingual zero-shot classification".
- Persian evidence: `fa` is among the 26 languages of the fine-tuning set (plus English), 105,000 pairs each. The card warns this set "was created using machine translation, which reduces the quality of the data", though "grammatical errors ... are less of an issue for zero-shot classification". The reported XNLI test table covers 15 languages and **does not include Persian**; the closest are Arabic 0.794, Hindi 0.769, Turkish 0.792, Russian 0.803. The card says hypotheses can be written in English for a text in another language ("10% ... English hypothesis paired with the premise in the other language").
- Serving: `onnx/model.onnx` and `onnx/model_quantized.onnx` ship in the repo. The standard recipe scores one premise-hypothesis pair per label, so a 17-label Input is 17 forward passes (batchable), against one for embeddings. Labels would be written as hypotheses ("this is a payment for groceries"), using the Category descriptions.
- Confidence: entailment probabilities from a softmax over labels (`multi_label=False`); usable for the gate, uncalibrated.
- Go: `hugot` documents text classification pipelines; I did not confirm that its zero-shot-text pipeline exists (its README lists zero-shot only for images). Doing the pair loop by hand over a text-classification pipeline is possible. Python sidecar with the `transformers` zero-shot pipeline is the simple route.
- Cost: about 2.4x the parameters of e5-small, times N labels, in one Input. Expect the slowest of the encoders on CPU (not measured).

### 3. Small instruct LLM via `llama-server`

`Qwen/Qwen3-1.7B` (1.7B, Apache 2.0), `Qwen/Qwen3-0.6B`, `Qwen/Qwen2.5-1.5B-Instruct` (1.54B, Apache 2.0). Official GGUF repos exist for all three (Qwen3-1.7B-GGUF has Q8_0 only; Qwen2.5-1.5B has q2_k to fp16).
- Persian evidence: Qwen3's blog language list includes Persian and the tech report says 119 languages and dialects (https://arxiv.org/abs/2505.09388, https://qwenlm.github.io/blog/qwen3/). The Qwen2.5-1.5B card lists 29 languages and does not name Persian. No Persian accuracy number for either at this size.
- Serving: `llama-server` (https://github.com/ggml-org/llama.cpp, `tools/server`) gives OpenAI-compatible `/v1/chat/completions`, `json_schema` and BNF `grammar` constraints (so the output is always one of the Category keys), and `n_probs` which returns top-N token log-probabilities per generated token. That makes a confidence available: constrain the answer to one key and read the probability of its first distinguishing token. Plain Go `net/http`, same as Metis.
- Cost: a 15 to 17 Category prompt with Persian hints is about 1,200 to 1,500 tokens on Jev's tokeniser (from `jev-sole-call.md`); a Qwen tokeniser's count for Persian is not documented. Prompt processing on CPU for that length is the dominant latency. A prompt prefix cache in `llama-server` would help since the Category list is fixed; not verified here.
- Weakness: generative LLM probabilities are not calibrated by design (Jev's selling point is RLCD calibration); a 0.6B to 1.7B model is the least likely of the three to follow "note wins over bank label" in Persian.

## Cost in complexity versus Jev on Metis

| | Jev on Metis | e5-small + head | mDeBERTa NLI | Qwen3 via llama-server |
|---|---|---|---|---|
| Money per Input | about $0.00006 | 0 (lab electricity) | 0 | 0 |
| New moving parts | none (existing HTTP call) | ONNX model + tokenizer in Go or a sidecar; a trained head file | same plus N-pass scoring | one more long-running service, GGUF, prompt |
| Labelled data | none | needed to train and calibrate | needed to calibrate | needed to calibrate |
| Persian evidence | 17/17 on the 18 cases (optimistic, see that doc) | adjacent only | adjacent only (fa trained, machine translated) | blog language list only |
| Confidence | built in, documented | head probability after calibration | softmax, uncalibrated | token probability, uncalibrated |
| Failure mode | Metis down, key, network | lab host down | lab host down | lab host down |

The saving is about one cent a month at 5 Inputs a day. The reasons to go local are availability of Metis or keeping bank text off a third party, not money.

## The one the Owner heard of

The ticket asks to find "the" model that runs without a GPU if it exists. Primary sources show several that do (all three families above run on CPU; the e5 and mDeBERTa repos ship int8 ONNX files made for it), but none named Jev-like for Persian. Candidate meant most likely: multilingual zero-shot NLI (mDeBERTa) or small embedding models. v1m's `laya-multilingual-onnx` (from `jev-v1m.md`) is the other name that matches "ONNX, no GPU, FA/AR/EN", but it is a hosted model with no published weights found. Which the Owner meant is unconfirmed.

## What a follow-up prototype ticket needs

- **Data:** the 18 cases in the gitignored `samples/forwarded-messages.txt` are enough for a smoke test, not to train or calibrate. Ask the Owner for a few hundred of their own past Inputs with the Category they would choose (even a couple of weeks of real use), keep a held-out set, and drop the personal details first. Include the cases Jev found hard: no note (5), loan with no matching Category (18), two-item note, free-form note only.
- **Pre-processing:** strip amounts, dates and the bank's own label text before embedding, as `jev-v1m.md` suggested for Jev. Feed the Owner's note on its own as well as the whole Input and compare.
- **Candidates to run in the first pass:** e5-small (int8 ONNX, nearest label then a logistic-regression head), e5-base and bge-m3 as accuracy ceilings for the embedding family, mDeBERTa NLI, Qwen3-1.7B Q8_0 through `llama-server`. Keep Python for the prototype; porting to Go is a later ticket.
- **Measure on `ali@lab`:** cold start, p50 and p95 latency per Input, resident RAM, with a fixed thread count; compare with Jev's p50 2.6 s and p95 6.4 s.
- **Score:** per-case top-1 against the acceptable alternatives used in `jev-sole-call.md`; the confidence at the 0.7 gate (which cases get flagged, and are they the unclear ones); option and label-order sensitivity; Persian versus English label text.
- **Decision rule to write down before running:** for example, a local model has to be within 1 acceptable miss of Jev on the held-out set and flag the unclear cases, otherwise stay on Jev.

## Not verified

- CPU latency and RAM of every candidate (no first-party numbers).
- Any Persian Category-style accuracy for any candidate; e5-small has no FaMTEB figure in the table I read.
- Whether `hugot` has a text zero-shot (NLI) pipeline, and ONNX export for SetFit.
- Hakim public availability and licence.
- Persian token counts under the Qwen tokenisers.
- Which model the Owner heard of.
