package cmd

import "testing"

func TestAuditProject(t *testing.T) {
	cases := map[string][]string{
		"team/backend": {"team/backend", "7"},
		"proj":         {"-p", "proj", "12"},
		"":             {"42"},
		"backend":      {"backend"},
	}
	for want, args := range cases {
		if got := auditProject(args); got != want {
			t.Errorf("auditProject(%v) = %q, want %q", args, got, want)
		}
	}
	if got := auditProject(nil); got != "" {
		t.Errorf("auditProject(nil) = %q", got)
	}
}

func TestMask(t *testing.T) {
	if got := mask("glpat-abcdefgh1234"); got != "****1234" {
		t.Errorf("mask = %q", got)
	}
	if got := mask("short"); got != "****" {
		t.Errorf("mask(short) = %q", got)
	}
}
