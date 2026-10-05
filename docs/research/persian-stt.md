# Persian speech-to-text options and prices (ticket #3)

Researched 2026-10-05. Question: which STT models can transcribe a Persian Voice note, through Metis or v1m, and what do they cost?

## Answer

- **v1m cannot do STT.** Its docs describe only Noul / Choice / Score decision primitives on text ("state") input. [v1m docs](https://v1m.ir/docs)
- **Metis can**, via an OpenAI-compatible endpoint: `POST https://api.metisai.ir/openai/v1/audio/transcriptions`, multipart form with `file` and `model` (docs example uses `whisper-1`; "outputs and settings follow OpenAI's models"). [Metis audio API docs](https://docs.metisai.ir/api/audio)
- Metis lists four dedicated transcription models, all billed per second of audio (USD, "all prices based on US dollar in free markets"):

| Model | Provider | Metis price per second | Per minute | Per 10 s note |
|---|---|---|---|---|
| `gpt-4o-mini-transcribe` | openai | $0.000055 | $0.0033 | $0.00055 |
| `whisper-1` | openai | $0.00011 | $0.0066 | $0.0011 |
| `gpt-4o-transcribe` | openai | $0.00011 | $0.0066 | $0.0011 |
| `scribe_v1` | elevenlabs | $0.0001221 | $0.0073 | $0.0012 |

  Source: Metis pricing JSON, `transcriptor` array: `https://api.metisai.ir/api/v1/meta/providers/pricing` (public, no key; this is the feed `https://docs.metisai.ir/pricing` renders client-side, see `pricing.tsx` chunk `4a25d6cc.337fb725.js`). The same entries also carry per-token prices (`gpt-4o-mini-transcribe` $1.375 in / $5.50 out per 1M tokens; `gpt-4o-transcribe` $2.75 / $11.00); `whisper-1` and `scribe_v1` are per-second only. Which of per-second vs per-token Metis actually bills for the OpenAI GPT-4o transcribe models is not stated.
- Metis prices are 1.1x OpenAI's list prices for the models checked: whisper-1 $0.006/min, gpt-4o-transcribe $2.50 / $10.00 per 1M tokens, gpt-4o-mini-transcribe $1.25 / $5.00 per 1M tokens. [OpenAI pricing](https://developers.openai.com/api/docs/pricing)
- **Audio-input chat models also exist on Metis** (usable as an STT-by-prompt fallback, not through the transcription endpoint): Gemini `input_audio_token` prices per 1M tokens: `gemini-2.5-flash-lite` $0.33, `gemini-2.0-flash` $0.77, `gemini-3.1-flash-lite` $0.55, `gemini-2.5-flash` $1.10, `gemini-3-flash-preview` $1.10; `gpt-4o-mini-audio-preview` $0.165 in / $0.66 out text-token price (audio-token price not listed). Same Metis pricing JSON, `llm` array. These would transcribe through a chat completion with a prompt, not a transcription endpoint, so output format is less predictable.

## Cost for this bot

Assumption (mine, not the Owner's): 30 Voice notes per day, 20 s each = 18,000 s/month.

| Model | Per month (Metis) |
|---|---|
| `gpt-4o-mini-transcribe` | about $0.99 |
| `whisper-1` | about $1.98 |
| `gpt-4o-transcribe` | about $1.98 |
| `scribe_v1` | about $2.20 |

All are negligible next to any plausible budget; price should not drive the choice. Scale linearly (about $0.0011 per 10 s note on the mid-priced models).

## Bale input format

Bale Voice messages are OGG/Opus; the Voice object has `file_id`, `file_unique_id`, `duration`, optional `mime_type`, `file_size`; bots can download files up to 20 MB via `getFile`. [Bale docs](https://docs.bale.ai/) So the bot can download the `.ogg` and POST it as-is, and `duration` gives the per-note cost in advance.

## Persian quality evidence

- Whisper large-v2 (the model behind `whisper-1`, per OpenAI's model naming; not confirmed on the Metis side): Persian WER on FLEURS **32.9%** (Table 13 of the Whisper paper). Roughly one word in three wrong, so Persian is usable but weak; expect amounts and merchant names to need the LLM step and the Owner's Follow-up. [Whisper paper, arXiv 2212.04356, Table 13](https://arxiv.org/abs/2212.04356). I read the Persian column by its position in the alphabetical language order of the PDF text extraction; confirm against the figure in the [Whisper README](https://github.com/openai/whisper) if the number is going into a decision.
- No primary Persian number found for `gpt-4o-transcribe`, `gpt-4o-mini-transcribe` or `scribe_v1` (see below).

## Recommendation (for the spec ticket, not yet a decision)

Candidates to send to the evaluation ticket, all reachable with one Metis key and one HTTP shape except ElevenLabs/Gemini details:

1. `gpt-4o-transcribe` (same price as `whisper-1`, newer; Persian quality unproven here).
2. `whisper-1` (baseline, the only one with a published Persian WER).
3. `scribe_v1` (ElevenLabs, marketed as broad-language; Persian support unverified).
4. `gpt-4o-mini-transcribe` (half price; cheapest dedicated option).
5. `gemini-2.5-flash-lite` audio input (about 6x cheaper per second than whisper on paper; needs a prompt and parse step).

Pick by running the Owner's sample voice notes through 1 to 4 (and 5 if cheap to wire) and comparing digit/amount accuracy, since the downstream pipeline needs numbers right. Price differences are cents per month.

## Could not verify

- Metis acceptance of `.ogg`/Opus on the transcription endpoint, and whether Metis exposes the `language` parameter (needs an API key; `api.metisai.ir` returns 401 without one).
- Whether Metis bills per second or per token for `gpt-4o-*transcribe`.
- Persian accuracy of `gpt-4o-transcribe`, `gpt-4o-mini-transcribe`, `scribe_v1`, and Gemini audio. OpenAI, ElevenLabs and Google doc pages returned HTTP 403 to automated fetch.
- Whether `whisper-1` on Metis is large-v2 (OpenAI's API alias; not stated by Metis).
- Gemini audio tokens per second (I recall 32/s from Google's docs but could not fetch them), so the Gemini cost is not computed.
- OpenAI's list shows a newer `gpt-transcribe` at $0.0045/min that is not in the Metis catalogue; Metis may add it.
- The Metis pricing feed is live data and may change after 2026-10-05.
