# Text input first, speech later

v1 accepts only Text notes: a forwarded bank message with an optional Owner note after it. Voice notes and the speech-to-text model choice are deferred to a later version.

The Owner mostly pays by card and finds forwarding the bank message convenient. The bank message already carries amount, Direction and date, so the Owner's note only adds a Category or description. Jev tags the Category from the note and bank text, so extra banks need no new parsing rules.

Consequences: no STT model, endpoint or cost in v1. Research stays in `docs/research/persian-stt.md` for when speech returns. Real samples live in the gitignored `samples/` folder.
