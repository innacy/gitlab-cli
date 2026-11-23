package cmd

import (
	"testing"

	"gitlab-ai/internal/models"
)

func TestIsIgnoredPath(t *testing.T) {
	ign := []string{"cnips/requirements", " CNIPS/testcase "}
	for path, want := range map[string]bool{
		"cnips/requirements":         true,
		"cnips/requirements/sub/one": true,
		"cnips/testcase":             true,
		"cnips/requirements-x":       false,
		"cnips/table-srv":            false,
	} {
		if got := isIgnoredPath(path, ign); got != want {
			t.Errorf("%s: got %v, want %v", path, got, want)
		}
	}
}

func TestAssignTarget(t *testing.T) {
	cases := []struct {
		name   string
		issue  models.Issue
		wantID int
		wantH  string
		wantOK bool
	}{
		{"creator", models.Issue{Author: "alice", AuthorID: 7}, 7, "alice", true},
		{"redirected", models.Issue{Author: redirectAuthorUsername, AuthorID: 9}, 1, "me", true},
		{"unknown author", models.Issue{}, 0, "", false},
	}
	for _, c := range cases {
		id, h, ok := assignTarget(c.issue, 1, "me")
		if id != c.wantID || h != c.wantH || ok != c.wantOK {
			t.Errorf("%s: got (%d,%q,%v), want (%d,%q,%v)", c.name, id, h, ok, c.wantID, c.wantH, c.wantOK)
		}
	}
}
