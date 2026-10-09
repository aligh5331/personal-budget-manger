package bot_test

import (
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bot/bottest"
)

func TestNonTextInputsGetTextOnlyReply(t *testing.T) {
	const want = "Text only for now. Send a bank message, with an optional note."
	cases := map[string]func(m *bale.Message){
		"voice":   func(m *bale.Message) { m.Voice = &bale.File{FileID: "v"} },
		"photo":   func(m *bale.Message) { m.Photo = []bale.File{{FileID: "p"}}; m.Caption = "ناهار" },
		"sticker": func(m *bale.Message) { m.Sticker = &bale.File{FileID: "s"} },
	}
	for name, attach := range cases {
		t.Run(name, func(t *testing.T) {
			h := bottest.New(t)
			m := h.OwnerMessage("")
			attach(m)
			h.SendMessage(m)

			sent := h.Sent()
			if len(sent) != 1 || sent[0].Text != want {
				t.Fatalf("sent %+v, want one %q", sent, want)
			}
		})
	}
}
