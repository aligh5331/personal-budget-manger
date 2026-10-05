# Bale Bot API: webhook, voice files, buttons, Go libraries

Research for ticket #5 (map #1). Primary source: https://docs.bale.ai/ (Persian, JS-rendered; read via raw HTML fetched 2026-10-05). Section names below refer to that page. Go library facts come from the libraries' own GitHub repos.

## Short answers

- Base URL `https://tapi.bale.ai/bot<token>/METHOD`; GET and POST; params as query string, urlencoded, JSON (except uploads) or multipart. Method names are case-insensitive. Replies are `{ok, result}` or `{ok:false, error_code, description, parameters?{retry_after}}`.
- Webhook: `setWebhook` takes only `url` (HTTPS). Ports 443 and 88. **No secret token parameter and no secret header are documented**, and `WebhookInfo` has only `url`. Hardening must come from a secret URL path, plus checking `from.id` against the Owner allowlist.
- Long polling: `getUpdates(offset, limit 1-100 default 100, timeout seconds)`. **No maximum timeout is documented** (the 10 min in the map is the Owner's plan, not a documented value). `deleteWebhook` is how you return to `getUpdates`; `getWebhookInfo` returns empty `url` when polling.
- Voice: `message.voice` documents only `file_id` and `file_unique_id`. Download: `getFile(file_id)` -> `file_path`, then GET `https://tapi.bale.ai/file/bot<token>/<file_path>`. Max 20 MB download; link valid one hour (call `getFile` again for a new one).
- Buttons: inline keyboards with `callback_data` (1-64 bytes); callback arrives as `update.callback_query`; **must call `answerCallbackQuery`** or the button stays in loading state. Edit with `editMessageText` / `editMessageReplyMarkup`; delete with `deleteMessage` (under 48 h old).
- Forwarded messages: `forward_from` (User), `forward_from_chat`, `forward_from_message_id`, `forward_date` (unix).
- Go: no official library. Docs say Telegram libraries work because the API is Telegram-based. Community Bale libraries exist but are small; calling HTTP directly is cheap for this bot's method set.

## Update delivery

- Two modes: `getUpdates` or webhook; both give the same `Update` JSON. Unreceived updates are stored on the server: "the last 2000 messages for 24 hours".
- `Update` fields: `update_id` (starts at 0, increasing; use to drop duplicates / restore order), and at most one of `message`, `edited_message`, `callback_query`, `pre_checkout_query`.
- `getUpdates.offset`: must be one more than the highest `update_id` seen; an offset above an `update_id` confirms it. Negative offset reads from the end of the queue and forgets all earlier updates. Recompute offset after every response.
- `timeout`: seconds to wait; empty response if nothing arrives.
- Webhook delivery: HTTPS POST of the JSON-serialized `Update` to the URL.
- `setWebhook` with an empty `url` disables the webhook (also documented as the way to remove it); `deleteWebhook` also does.
- Not documented: webhook retry policy, expected response code/timeout, whether `getUpdates` errors while a webhook is set (Telegram does; Bale's page says to call `deleteWebhook` before using `getUpdates` again), IP ranges Bale sends from.

## Messages and voice

- `Message`: `message_id`, `from` (User, optional), `date` (unix), `chat`, `text`, `caption`, `voice`, `forward_*`, `reply_to_message`, `edit_date`, `entities`, etc.
- `Voice`: `file_id`, `file_unique_id` only on the docs page. No `duration`, `mime_type` or `file_size` listed. `File.file_size` is optional, so check size after `getFile`, not before.
- `File`: `file_id`, `file_unique_id`, `file_size` (optional), `file_path` (optional).
- `sendVoice` (outgoing): audio/ogg; the "sending by URL" note says voice over 1 MB is sent as a file; irrelevant for inbound.
- `sendMessage`/`editMessageText` text: 1-4096 characters.
- `sendChatAction` shows status for at most 6 seconds (useful while STT runs; send repeatedly if slow).

## Buttons and editing

- `InlineKeyboardMarkup.inline_keyboard`: rows of `InlineKeyboardButton`. A button uses exactly one of `url`, `callback_data` (1-64 bytes), `web_app`, `copy_text`.
- `CallbackQuery`: `id`, `from`, `message` (optional; content/date missing for very old messages), `data`. Doc warns the message may have no button with that data, so validate `data`.
- `answerCallbackQuery(callback_query_id)` is required even with no other params. Feature was added to Bale clients in Khordad 1404; if `callback_query.id` starts with `1`, the client is old and unsupported, so fall back to a normal message.
- `editMessageText(chat_id, message_id, text 1-4096, reply_markup?)`, `editMessageCaption`, `editMessageReplyMarkup`.
- `deleteMessage(chat_id, message_id)`: only if sent under 48 h ago; bots can delete incoming messages in private chats and own outgoing messages.
- `ReplyKeyboardMarkup` / `ReplyKeyboardRemove` also exist.
- I did not find `setMyCommands`/`BotCommand` on the page (grep of page text), so a command menu may not be settable by API. Not confirmed absent from Bale generally.

## Rate limits

- Standard API: limited, computed "interactively": a bot's send capacity depends on how much users interact with it in private chats. No numbers published. 429-style errors carry `parameters.retry_after` (seconds).
- Business API (`https://tapi.bale.ai/business/bot<TOKEN>/METHOD`) has a separate, higher, paid quota but only a few methods and private chats via `chat_id`. Not needed for a single-user bot.

## Go libraries

| Library | Last push | Notes |
|---|---|---|
| github.com/AbolfazlZarei-dev/ParsBale-bot-go | 2026-08-11 | Bale-specific, MIT, Go 1.21, router/FSM/middleware, webhook + polling, 10 stars. Most recent, young, heavy framework. |
| github.com/mikhikhi/baleBot | 2025-01-04 | MIT, 0 stars. |
| github.com/rbehzadan/go-bale-bot-api | 2023-06-19 | Fork of go-telegram-bot-api, 0 stars. |
| github.com/GhiaC/bale-bot-api | 2019-12-17 | Fork of go-telegram-bot-api with `tapi.bale.ai` hardcoded, stale, 8 stars. |
| github.com/arashrahimi46/bale-bot-go-api | 2020-02-12 | Same lineage, stale. |
| github.com/go-telegram-bot-api/telegram-bot-api (v5) | 2024-08-14 | Has `NewBotAPIWithAPIEndpoint(token, endpoint)` so a Bale endpoint can be set. Telegram types lack Bale-only fields. |
| github.com/go-telegram/bot | 2026-10-04 | Active; has `WithServerURL`. |

Not verified: whether the Telegram libraries build the file-download URL correctly for Bale's `/file/bot<token>/` path, and whether any library handles Bale quirks (voice without duration, old-client callback ids). Recommendation for the spec: plain `net/http` client for roughly 8 methods (`getUpdates`, `setWebhook`, `deleteWebhook`, `getWebhookInfo`, `sendMessage`, `editMessageText`, `answerCallbackQuery`, `getFile` + file GET, `deleteMessage`, `sendChatAction`) with hand-written structs; this removes a dependency on small, unmaintained libraries and keeps the polling/webhook switch under our control.

## Could not verify

- Max `getUpdates` timeout; whether Bale cuts idle connections before 10 min.
- Webhook retry behavior, timeouts, source IPs, any secret header.
- Concrete rate-limit numbers.
- Voice codec/duration (expected OGG/Opus by analogy with Telegram; not documented in inbound form).
- Behavior of `forward_from` for users who hide their identity.
- Library behavior against live Bale (no bot token was used; nothing was called).
