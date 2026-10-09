// Package extract reads an Input with the extraction LLM. The bot core
// depends only on the Extractor interface; Metis is the production
// implementation and extractfake the test double.
//
// A Result is the model's raw reading. It is not trusted: package inputrules
// checks and normalizes it against the Input text before anything is saved.
package extract

import (
	"context"
	"time"
)

// Extractor reads one Input. now is used for "today" in the prompt.
type Extractor interface {
	Extract(ctx context.Context, input string, now time.Time) (Result, error)
}

// Directions the model may return.
const (
	DirOut       = "out"
	DirIn        = "in"
	DirAmbiguous = "ambiguous"
)

// Units the model may return.
const (
	UnitRial  = "rial"
	UnitToman = "toman"
)

// Result is the model's reading of one Input. JSON nulls decode to zero
// values, so an empty string or a zero amount means "not given".
type Result struct {
	// IsTransaction is false for ads, bill notices, future-tense notices
	// and chit-chat.
	IsTransaction bool `json:"is_transaction"`
	// Note is the Owner's own words as the model copied them.
	Note string `json:"note"`
	// Transactions has one entry per bank message, or one from the note
	// when there is no bank message.
	Transactions []Item `json:"transactions"`
}

// Item is one money movement as the model read it.
type Item struct {
	// Amount as printed, digits only (0 when missing).
	Amount int64 `json:"amount"`
	// AmountUnit is UnitRial or UnitToman.
	AmountUnit string `json:"amount_unit"`
	// Direction is DirOut, DirIn or DirAmbiguous.
	Direction string `json:"direction"`
	// Date is Jalali "YYYY/MM/DD" ("" when missing).
	Date string `json:"date"`
	// Time is "HH:MM" ("" when missing).
	Time string `json:"time"`
	// BankLabel is the bank's own label or hashtag ("" when missing).
	BankLabel string `json:"bank_label"`
	// Confidence in amount and direction, 0..1.
	Confidence float64 `json:"confidence"`
}
