package storage

import "time"

// ReportLine is one Category's total for a Report: the sum of the
// non-flagged Transactions of one Direction.
type ReportLine struct {
	CategoryID int64
	// CategoryName is kept even when the Category is archived.
	CategoryName string
	// Direction is DirectionOut or DirectionIn.
	Direction string
	Total     int64
}

// ReportSummary is what a Report is built from, for one time range.
type ReportSummary struct {
	// Lines are the expense and income totals by Category, in no set order.
	Lines []ReportLine
	// InternalToman and InternalCount cover internal transfers, which stay
	// out of the totals.
	InternalToman int64
	InternalCount int
	// FlaggedCount is the number of Flagged transactions in the range;
	// they are in no other field.
	FlaggedCount int
}

// ReportRange is the half-open time range [From, To) of a Report.
type ReportRange struct {
	From, To time.Time
}
