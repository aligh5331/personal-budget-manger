// Package inputrules turns the extraction model's reading of an Input into
// Transaction drafts. It is pure: no I/O, no clock (the caller passes now),
// so every rule is covered by table tests.
//
// The model is not trusted. Each rule re-checks what it returned against the
// Input text:
//
//   - amounts must appear in the text (and not only as the balance);
//     bank amounts are rial and become toman (÷10, rounded); note amounts
//     are colloquial toman, and the bank amount wins; see noteamount.go;
//
//   - Direction comes from the sign after the amount, then the bank keyword,
//     then the Owner's note, then the model; see direction.go;
//
//   - the bank date wins, with its year inferred when not printed; the time
//     is kept only when printed; then a relative date in the note
//     (دیروز); see date.go and reldate.go;
//
//   - the description is only the Owner's own words; see note.go.
//
//   - at most MaxBankMessages bank messages per Input, and an equal out/in
//     pair at the same minute is one internal transfer; see batch.go.
//
// Apply is the only entry point. Later rules extend it here, behind the
// same seam.
package inputrules

import (
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/extract"
)

// Direction of a draft.
type Direction string

// Directions. Ambiguous drafts need a Direction Follow-up.
const (
	Out       Direction = "out"
	In        Direction = "in"
	Ambiguous Direction = "ambiguous"
	// Internal is a transfer between the Owner's own accounts (batch.go).
	Internal Direction = "internal"
)

// Field names what a Follow-up must ask for.
type Field string

// Follow-up fields. A draft needs at most one.
const (
	FieldNone      Field = ""
	FieldAmount    Field = "amount"
	FieldDirection Field = "direction"
)

// Input is everything the rules need.
type Input struct {
	// Text is the raw Input as the Owner sent it.
	Text string
	// Extraction is the model's reading of Text.
	Extraction extract.Result
	// Now is the current time; "today" is taken in Asia/Tehran.
	Now time.Time
}

// Outcome is the result of Apply.
type Outcome struct {
	// IsTransaction is false when the Input is not a money movement (or the
	// model found none); Drafts is then empty.
	IsTransaction bool
	// Drafts holds one draft per money movement, in Input order.
	Drafts []Draft
	// TooMany is true when the Input holds more than MaxBankMessages bank
	// messages; Drafts is then empty and nothing may be saved.
	TooMany bool
}

// Draft is a Transaction before it is saved.
type Draft struct {
	// AmountToman is valid only when HasAmount.
	AmountToman int64
	HasAmount   bool
	Direction   Direction
	// OccurredAt is in Asia/Tehran. Without a printed time it is midnight
	// of the bank date, or Now when the Input has no date.
	OccurredAt time.Time
	// HasTime is true when the bank message printed a time of day.
	HasTime bool
	// Description is the Owner's own words only ("" when none).
	Description string
	// BankLabel is the bank's own label or hashtag ("" when none).
	BankLabel string
	// MessageText is what the Categorizer reads: this draft's bank message
	// plus the shared note, as the Owner wrote them (the whole Input when
	// the Input is one message, or the message cannot be told apart).
	MessageText string
	// FollowUp is the one field that must be asked before the draft is
	// complete, or FieldNone.
	FollowUp Field
}

// Apply runs every Input rule.
func Apply(in Input) Outcome {
	ex := in.Extraction
	if !ex.IsTransaction || len(ex.Transactions) == 0 {
		return Outcome{}
	}
	if tooMany(len(ex.Transactions)) {
		return Outcome{IsTransaction: true, TooMany: true}
	}
	now := in.Now.In(clock.Tehran())
	d := parseDoc(in.Text)
	desc := description(d, in.Text, ex.Note)

	out := Outcome{IsTransaction: true}
	seen := map[int64]int{} // amount -> items so far, to pair repeats with later occurrences
	for _, it := range ex.Transactions {
		dr := Draft{Description: desc, BankLabel: bankLabel(in.Text, it.BankLabel)}

		tok, toman, found := readAmount(d, desc, it, seen[it.Amount])
		seen[it.Amount]++
		if found {
			dr.AmountToman, dr.HasAmount = toman, true
		}
		dr.MessageText = messageText(in.Text, d, tok, found, desc)
		dr.Direction = direction(d, tok, found, desc, it)
		dr.OccurredAt, dr.HasTime = occurredAt(d, it, now, desc)

		switch {
		case !dr.HasAmount:
			dr.FollowUp = FieldAmount
		case dr.Direction == Ambiguous:
			dr.FollowUp = FieldDirection
		}
		out.Drafts = append(out.Drafts, dr)
	}
	out.Drafts = mergeInternal(out.Drafts)
	return out
}

// bankLabel keeps the model's bank label only if it is in the text.
func bankLabel(text, label string) string {
	if !contains(text, label) {
		return ""
	}
	return oneLine(Normalize(label))
}
