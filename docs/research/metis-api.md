# Metis API: compatibility, endpoints, models, prices

Research for ticket #2. Retrieved 2026-10-05.

Sources (all first-party):
- Docs (Docusaurus, server-rendered except the pricing page; plain `curl` of `http://docs.metisai.ir/<path>/` with trailing slash works): `/api/start-api/`, `/api/audio/`, `/api/wrapper/`, `/api/wrapper/openai/`, `/api/wrapper/deepseek/`, `/api/wrapper/typesafe/`, `/api/wrapper/gemini/`, `/api/providers/`. Page list from `http://docs.metisai.ir/sitemap.xml`.
- Pricing page https://docs.metisai.ir/pricing is client-rendered. Its JS bundle (`/assets/js/4a25d6cc.337fb725.js`) fetches `GET https://api.metisai.ir/api/v1/meta/providers/pricing` (public, no key, returns JSON). Every price below comes from that endpoint, which is the same data the pricing page shows.

## Compatibility and endpoints

- Base URL: `https://api.metisai.ir`. Auth: `Authorization: Bearer <METIS_API_KEY>` (start-api).
- OpenAI-compatible: base URL `https://api.metisai.ir/openai/v1` replaces `api.openai.com`; docs say it works for Chat, Embedding, Batch and the other OpenAI services (wrapper/openai). Use with any OpenAI client or plain HTTP: `POST /openai/v1/chat/completions`.
- Speech-to-text, OpenAI format: `POST https://api.metisai.ir/openai/v1/audio/transcriptions`, multipart form with `file` and `model` (api/audio). Docs say all inputs and outputs follow OpenAI's. Doc examples use `whisper-1` and `gpt-4o-transcribe`.
- DeepSeek, OpenAI format: `https://api.metisai.ir/deepseek/v1/chat/completions`. Models `deepseek-v4-flash`, `deepseek-v4-pro`. Old ids `deepseek-chat` and `deepseek-reasoner` are deprecated and silently routed to v4-flash, so send the id explicitly (wrapper/deepseek).
- Grok, OpenAI format: `https://api.metisai.ir/api/v1/wrapper/grok/chat/completions` (wrapper).
- Gemini native: base `https://api.metisai.ir`, `x-goog-api-key` header, `/v1beta/models/<model>:generateContent` (wrapper/gemini). Anthropic also has a wrapper page (`/api/wrapper/anthropic`, not read in detail).
- Jev (TypeSafe System One): `POST https://api.metisai.ir/typesafe/v1/systemone`, Bearer Metis key, model `jev-latest` (the only supported model). Body: `state` (input text), `model`, `questions` map. Question types seen in the example: `choice` (with `criteria` map), `score` (criteria list), `noul`. Response: `answers.<name>` with `choice`, `confidence`, `probabilities`; plus `usage.input_tokens/output_tokens` (wrapper/typesafe). Full question schema lives in TypeSafe's own quickstart, which is not on the Metis docs.
- Model list endpoint: `GET /api/v1/meta` (providers doc; the sample output on that page is old).
- Direct-wrapper mode loses Metis bots and function calling; "full coverage of every model feature is not guaranteed" (wrapper/openai). Metis also has its own bot/session API, not needed here.
- Live probe without a key: `POST /openai/v1/audio/transcriptions` and `/openai/v1/chat/completions` return HTTP 401, so the routes exist. The 401 body says to send the key as `Authorization: Bearer tpsg-<key>` or `x-api-key`; this looks like a generic gateway message and the doc examples use plain Bearer. Check the real key format when the Owner creates one.

## Prices

Source: pricing JSON above. All values are listed with `currency: USD`. Per 1M tokens unless stated.

### Speech-to-text (`transcriptor`, billed per second of audio)

| Model | Price per second | Per minute |
|---|---|---|
| gpt-4o-mini-transcribe | $0.000055 (+ token fields input $1.375/1M, output $5.5/1M listed) | $0.0033 |
| whisper-1 | $0.00011 | $0.0066 |
| gpt-4o-transcribe | $0.00011 (+ token fields input $2.75/1M, output $11/1M listed) | $0.0066 |
| elevenlabs scribe_v1 | $0.0001221 | $0.0073 |

Unclear whether gpt-4o-* transcribe bill per second or per token, since both fields are present. Whether scribe_v1 is reachable through the OpenAI-format transcriptions endpoint is not documented.

### LLM candidates for parsing and Category step (input / output per 1M tokens)

| Model | Input | Output |
|---|---|---|
| jev-latest (typesafe) | $0.046 | $0 |
| gpt-5-nano | $0.055 | $0.44 |
| gpt-4.1-nano | $0.11 | $0.44 |
| gemini-2.5-flash-lite | $0.11 | $0.44 |
| gpt-6-luna | $0.11 | $0.55 |
| deepseek-v4-flash | $0.165 | $0.66 |
| gpt-4o-mini | $0.165 | $0.66 |
| gpt-5.6-luna | $0.22 | $1.32 |
| grok-4-fast / grok-4-1-fast | $0.22 | $0.55 |
| gemini-3.1-flash-lite | $0.275 | $1.65 |
| gpt-5-mini | $0.275 | $2.20 |
| gemini-2.5-flash | $0.33 | $2.75 |
| gpt-4.1-mini | $0.44 | $1.76 |
| deepseek-v4-pro | $0.726 | $2.178 |
| claude-haiku-4-5 | $1.10 | $5.50 |

The full list has about 125 LLM entries (Claude, GPT-5.x/6.x, Gemini 3.x, Grok, GLM, Kimi, MiMo). It includes the Anthropic ids `claude-sonnet-5-5` ($2.20 / $11.00) and `claude-opus-5-5` ($4.40 / $22.00). To re-pull: `curl https://api.metisai.ir/api/v1/meta/providers/pricing`.

### Rough per-Input cost

Assumptions are mine, not measured: a 20 s Voice note, and an LLM call of about 600 input and 150 output tokens.
- STT 20 s: whisper-1 $0.0022; gpt-4o-mini-transcribe about $0.0011.
- LLM call: deepseek-v4-flash about $0.0002; gpt-4.1-nano about $0.00013.
- Jev call, 400 input tokens: about $0.00002.
So an Input costs roughly 0.1 to 0.3 cent, dominated by STT.

## Not verified

- Whether the Persian transcription quality of whisper-1, gpt-4o-mini-transcribe or gpt-4o-transcribe is acceptable. Needs Owner voice samples (evaluation ticket).
- Whether the OpenAI wrapper accepts `language`, `prompt`, `response_format` on transcriptions, and the audio file size limit and accepted formats (Bale voice notes are OGG/Opus). Docs only say "same as OpenAI".
- Whether chat completions through Metis support JSON mode / structured outputs (`response_format`) for each model.
- Whether `/openai/v1/chat/completions` serves non-OpenAI models, or those must use their own wrapper URLs.
- Rate limits, and how the USD price converts to the Owner's toman balance; the docs do not say.
- Real API key format (see 401 note above); no key was available, so no authenticated call was made.
- Full TypeSafe System One request schema (only the Metis doc example was read).
- The Anthropic wrapper page content.
