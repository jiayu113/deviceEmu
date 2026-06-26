package device

import (
	"errors"
	"testing"
)

func TestParseCommand(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr error
		action  string
	}{
		{"ok call", `{"action":"call","target":"sip:x@1.2.3.4"}`, nil, "call"},
		{"ok with req", `{"request_id":"r1","action":"report_now"}`, nil, "report_now"},
		{"bad json", `{not json`, errInvalidJSON, ""},
		{"no action", `{"target":"x"}`, errNoAction, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseCommand([]byte(c.in))
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err=%v want=%v", err, c.wantErr)
			}
			if err == nil && got.Action != c.action {
				t.Fatalf("action=%s want=%s", got.Action, c.action)
			}
		})
	}
}
