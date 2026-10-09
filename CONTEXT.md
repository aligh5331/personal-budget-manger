# Personal Budget Manager

A single-user budget tracker. The owner records money spent or received by sending text to a Bale bot, mostly forwarded bank messages with a short note. Voice input is planned for a later version.

## Language

**Owner**:
The one person allowed to use the bot. Messages from any other Bale account are ignored.
_Avoid_: User, account holder

**Voice note**:
A Bale voice message from the Owner, in Persian, describing one Transaction. Not part of v1 (see `docs/adr/0001-text-input-first.md`).
_Avoid_: Audio, recording

**Text note**:
A Bale text message from the Owner describing a Transaction. In v0.1 it is a forwarded bank message (personal details stripped by the Owner) followed by an optional note of the Owner's own words, such as a category word or a short description. The note may contain typos and wins over the bank's own label. It may also be the Owner's words alone, with no bank message.
_Avoid_: Message, SMS (the SMS is the content, not the thing the bot receives)

**Input**:
A Text note in v1. A Voice note is a second kind of Input, planned for later.

**Follow-up**:
The bot's question when an Input lacks something a Transaction needs. Asked at most once per Transaction.
_Avoid_: Clarification, retry

**Flagged transaction**:
A Transaction saved incomplete because the Owner's Follow-up went unanswered, was overtaken by a new Input, or still left gaps. A Transaction saved as "Uncategorized" with no Follow-up is not flagged. It shows up in reports as flagged, outside the Category totals, until the Owner fixes it.
_Avoid_: Draft, pending, error

**Transaction**:
One record of money spent or received: amount, direction, Category, description, date. The description is the Owner's own words from the note, never the bank's label.
_Avoid_: Invoice, expense (an expense is a Transaction whose direction is out), entry

**Direction**:
Whether a Transaction is money out (expense), money in (income), or an internal transfer between the Owner's own accounts. Internal transfers are saved but excluded from income and expense totals.

**Category**:
A label from the Owner's editable list that groups Transactions for reports. Each is either expense or income, and can be archived but never deleted. Internal transfers have none.
_Avoid_: Tag, type

**Undo**:
Removing the Transaction the bot just saved, via a button on its confirmation message.
_Avoid_: Cancel, rollback

**Report**:
A summary of Transactions by Category for a month, this one or last.
_Avoid_: Statement, dashboard

## Money

All amounts are whole **toman**. Dates are shown in the Jalali calendar.
