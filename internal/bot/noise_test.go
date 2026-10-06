package bot_test

import (
	"context"
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
	"github.com/aligh5331/personal-budget-manger/internal/extract"
)

func TestNoiseIsRejectedWithNoTransactionFound(t *testing.T) {
	// Made-up examples of the noise kinds the model must reject.
	tests := []struct {
		name, text string
		reading    extract.Result
	}{
		{"bill notice", "قبض برق شما به مبلغ 450,000 ریال صادر شد. شناسه قبض: 1234567 مهلت پرداخت: 1405/07/20",
			extract.Result{IsTransaction: false}},
		{"ad", "با کارت بانک ملی تا ۳۰٪ تخفیف بگیرید! همین حالا اقدام کنید", extract.Result{IsTransaction: false}},
		{"one-time password", "رمز یکبار مصرف شما: 482913\nمبلغ: 1,200,000 ریال", extract.Result{IsTransaction: false}},
		{"future tense", "مبلغ 5,000,000 ریال فردا به حساب شما واریز خواهد شد", extract.Result{IsTransaction: false}},
		{"model says yes but reads no money movement", "سلام، خوبی؟", extract.Result{IsTransaction: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := bottest.New(t)
			h.Extractor.Return(tc.reading)

			h.SendText(tc.text)

			sent := h.Sent()
			if len(sent) != 1 {
				t.Fatalf("sent %d messages, want one reply", len(sent))
			}
			assertContains(t, sent[0].Text, "No transaction found")
			if sent[0].ReplyMarkup != nil {
				t.Errorf("reply has buttons: %+v", sent[0].ReplyMarkup)
			}
			if _, ok, _ := h.Store.TransactionByID(context.Background(), 1); ok {
				t.Error("a Transaction was saved")
			}
		})
	}
}
