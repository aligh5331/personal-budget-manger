package inputrules

// MaxBankMessages is the most bank messages one Input may hold. Above it
// nothing is saved and the Owner is asked to resend smaller batches, so a
// bad paste never creates a pile of wrong Transactions.
const MaxBankMessages = 5

// tooMany reports whether the model read more bank messages than allowed.
func tooMany(items int) bool { return items > MaxBankMessages }

// mergeInternal turns each equal out/in pair at the same date and minute into
// one internal transfer: money moved between the Owner's own accounts. The
// merged draft takes the place of the pair's first draft and keeps the
// amount once. Only drafts with an amount, a known Direction and a printed
// time take part; each draft pairs at most once, in Input order.
func mergeInternal(drafts []Draft) []Draft {
	used := make([]bool, len(drafts))
	out := make([]Draft, 0, len(drafts))
	for i, d := range drafts {
		if used[i] {
			continue
		}
		if j := internalPartner(drafts, used, i); j >= 0 {
			used[j] = true
			d.Direction = Internal
			if d.BankLabel == "" {
				d.BankLabel = drafts[j].BankLabel
			}
		}
		out = append(out, d)
	}
	return out
}

// internalPartner returns the index of the first unused draft after i that
// forms an internal pair with drafts[i], or -1.
func internalPartner(drafts []Draft, used []bool, i int) int {
	a := drafts[i]
	if !pairable(a) {
		return -1
	}
	for j := i + 1; j < len(drafts); j++ {
		b := drafts[j]
		if used[j] || !pairable(b) || b.Direction == a.Direction {
			continue
		}
		if a.AmountToman == b.AmountToman && a.OccurredAt.Equal(b.OccurredAt) {
			return j
		}
	}
	return -1
}

func pairable(d Draft) bool {
	return d.HasAmount && d.HasTime && d.FollowUp == FieldNone && (d.Direction == Out || d.Direction == In)
}
