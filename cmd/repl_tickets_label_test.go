package cmd

import (
	"slices"
	"testing"
)

func TestParseLabelSuggestion(t *testing.T) {
	avail := []string{"bug", "Backend", "UI", "docs"}
	tests := []struct {
		name      string
		reply     string
		wantLbls  []string
		wantNew   string
		wantColor string
		wantErr   bool
	}{
		{"exact", `{"labels":["bug","UI"],"reason":"x"}`, []string{"bug", "UI"}, "", "", false},
		{"fenced + case-insensitive", "```json\n{\"labels\":[\"backend\"]}\n```", []string{"Backend"}, "", "", false},
		{"hallucinated dropped", `{"labels":["bug","made-up"]}`, []string{"bug"}, "", "", false},
		{"capped at 3", `{"labels":["bug","Backend","UI","docs"]}`, []string{"bug", "Backend", "UI"}, "", "", false},
		{"new label ignored when match", `{"labels":["docs"],"new_label":"x"}`, []string{"docs"}, "", "", false},
		{"new label with color", `{"labels":[],"new_label":"perf","color":"#FF0000"}`, nil, "perf", "#FF0000", false},
		{"bad color defaults", `{"labels":["nope"],"new_label":"perf","color":"red"}`, nil, "perf", defaultNewLabelColor, false},
		{"garbage", `not json`, nil, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLabelSuggestion(tt.reply, avail)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !slices.Equal(got.Labels, tt.wantLbls) || got.NewLabel != tt.wantNew || (tt.wantNew != "" && got.Color != tt.wantColor) {
				t.Errorf("got %+v", got)
			}
		})
	}
}
