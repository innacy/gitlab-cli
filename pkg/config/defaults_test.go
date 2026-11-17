package config

import "testing"

func TestDefaultReviewSections(t *testing.T) {
	cfg := loadFromYAML(t, "gitlab:\n  base_url: \"https://x/\"\n")
	if n := len(cfg.Review.Template.Sections); n != 6 || cfg.Review.Template.Sections[0].Name != "Summary" || !cfg.Review.Template.Sections[0].Required {
		t.Fatalf("default sections = %+v", cfg.Review.Template.Sections)
	}
	for _, w := range Validate(cfg).Warnings {
		if w == "review.template.sections is empty — mr-review will not work until configured" {
			t.Error("still warns about empty review sections")
		}
	}
	custom := loadFromYAML(t, "review:\n  template:\n    sections:\n      - name: Only\n        prompt: p\n")
	if len(custom.Review.Template.Sections) != 1 {
		t.Errorf("config sections should replace defaults, got %d", len(custom.Review.Template.Sections))
	}
}
