package cmd

import "testing"

func TestParseIssueRef(t *testing.T) {
	cases := []struct {
		in      string
		project string
		iid     int
		ok      bool
	}{
		{"https://gitlab.widas.de/cnips/ops/-/issues/42", "cnips/ops", 42, true},
		{"https://gitlab.widas.de/cnips/ops/-/issues/42#note_1", "cnips/ops", 42, true},
		{"cnips/ops#7", "cnips/ops", 7, true},
		{"#7", "", 0, false},
		{"7", "", 0, false},
		{"ops#7", "", 0, false},
		{"cnips/ops#x", "", 0, false},
	}
	for _, c := range cases {
		p, n, ok := parseIssueRef(c.in)
		if p != c.project || n != c.iid || ok != c.ok {
			t.Errorf("parseIssueRef(%q) = %q,%d,%v", c.in, p, n, ok)
		}
	}
}
