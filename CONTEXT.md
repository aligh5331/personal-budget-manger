# Personal Budget Manager

A single-user budget tracker. The owner records money spent or received by talking to a Bale bot, mostly with Persian voice notes.

## Language

**Owner**:
The one person allowed to use the bot. Messages from any other Bale account are ignored.
_Avoid_: User, account holder

**Voice note**:
A Bale voice message from the Owner, in Persian, describing one Transaction.
_Avoid_: Audio, recording

**Text note**:
A Bale text message from the Owner describing a Transaction. It may be a forwarded or pasted bank deposit SMS, with or without extra words from the Owner.
_Avoid_: Message, SMS (the SMS is the content, not the thing the bot receives)

**Input**:
Either a Voice note or a Text note; the two ways the Owner reports a Transaction.

**Follow-up**:
The bot's question when an Input lacks something a Transaction needs. Asked at most once per Transaction.
_Avoid_: Clarification, retry

**Flagged transaction**:
A Transaction saved incomplete because the Owner's Follow-up went unanswered or still left gaps. It shows up in reports as flagged until the Owner fixes it.
_Avoid_: Draft, pending, error

**Transaction**:
One record of money spent or received: amount, direction, Category, description, date.
_Avoid_: Invoice, expense (an expense is a Transaction whose direction is out), entry

**Direction**:
Whether a Transaction is money out (expense) or money in (income).

**Category**:
A label from the Owner's editable list that groups Transactions for reports.
_Avoid_: Tag, type

**Undo**:
Removing the Transaction the bot just saved, via a button on its confirmation message.
_Avoid_: Cancel, rollback

**Report**:
A summary of Transactions by Category for a month, this one or last.
_Avoid_: Statement, dashboard

## Money

All amounts are whole **toman**. Dates are shown in the Jalali calendar.
