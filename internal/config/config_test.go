package config_test

import (
	"strings"
	"testing"

	"github.com/aligh5331/personal-budget-manger/internal/config"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	c, err := config.Load(env(map[string]string{
		"BALE_BOT_TOKEN": "123:abc",
		"OWNER_BALE_ID":  "424242",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.OwnerID != 424242 || c.ListenAddr != ":8080" || c.DataDir != "./data" ||
		c.ReportsTZ.String() != "Asia/Tehran" || c.ModeDefault != "" {
		t.Fatalf("defaults wrong: %+v", c)
	}
}

func TestLoadRejects(t *testing.T) {
	base := map[string]string{"BALE_BOT_TOKEN": "t", "OWNER_BALE_ID": "1"}
	cases := map[string]map[string]string{
		"missing token":   {"OWNER_BALE_ID": "1"},
		"missing owner":   {"BALE_BOT_TOKEN": "t"},
		"owner not a num": {"BALE_BOT_TOKEN": "t", "OWNER_BALE_ID": "me"},
		"bad mode":        with(base, "MODE_DEFAULT", "carrier-pigeon"),
		"bad tz":          with(base, "TZ_REPORTS", "Mars/Olympus"),
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := config.Load(env(m)); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestSecretsAreNotLogged(t *testing.T) {
	c, err := config.Load(env(map[string]string{
		"BALE_BOT_TOKEN":      "123:supersecrettoken",
		"OWNER_BALE_ID":       "1",
		"LLM_API_KEY":         "tpsg-secretkey",
		"WEBHOOK_SECRET_PATH": "a-very-long-secret-path-of-32-chars-plus",
	}))
	if err != nil {
		t.Fatal(err)
	}
	s := c.LogValue().String()
	for _, secret := range []string{"supersecrettoken", "secretkey", "a-very-long-secret-path"} {
		if strings.Contains(s, secret) {
			t.Errorf("log value leaks %q: %s", secret, s)
		}
	}
}

func with(m map[string]string, k, v string) map[string]string {
	out := map[string]string{k: v}
	for kk, vv := range m {
		if kk != k {
			out[kk] = vv
		}
	}
	return out
}
