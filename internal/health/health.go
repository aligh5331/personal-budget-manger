// Package health serves /healthz from a list of named checks.
package health

import (
	"encoding/json"
	"net/http"
)

// Check is one named part of the bot ("db", "updates", ...).
type Check struct {
	Name string
	Func func() error
}

type response struct {
	Status  string            `json:"status"`
	Failing map[string]string `json:"failing,omitempty"`
}

// Handler returns 200 {"status":"ok"} when every check passes, else 503
// {"status":"fail","failing":{"<name>":"<error>"}}. That the handler answers
// at all is the "process is up" check.
func Handler(checks ...Check) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := response{Status: "ok"}
		for _, c := range checks {
			if err := c.Func(); err != nil {
				if resp.Failing == nil {
					resp.Failing = map[string]string{}
				}
				resp.Failing[c.Name] = err.Error()
			}
		}
		code := http.StatusOK
		if resp.Failing != nil {
			resp.Status = "fail"
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(resp)
	})
}
