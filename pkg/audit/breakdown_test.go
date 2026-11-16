package audit

import (
	"testing"
	"time"
)

func TestBreakdowns(t *testing.T) {
	rec, _ := OpenInMemory()
	defer rec.Close()
	add := func(cmd, project string, ok bool) {
		id, _ := rec.Record(Event{Command: cmd, Category: CategoryRead, Project: project, Success: true})
		rec.Finish(id, 100, ok)
	}
	add("mr-status", "a", true)
	add("mr-status", "a", false)
	add("pipeline", "b", true)
	add("list", "", true)
	rec.Record(Event{Command: "mr-status", Category: CategoryFileWrite, Project: "a", Success: true}) // effect, ignored

	today := time.Now().Format("2006-01-02")
	cmds, err := rec.CommandBreakdown(today, today)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 3 || cmds[0] != (CommandRow{"mr-status", 2, 1, 100}) {
		t.Errorf("CommandBreakdown = %+v", cmds)
	}

	projects, _ := rec.ProjectBreakdown(today, today, 10)
	if len(projects) != 2 || projects[0] != (ProjectRow{"a", 2}) || projects[1] != (ProjectRow{"b", 1}) {
		t.Errorf("ProjectBreakdown = %+v", projects)
	}

	events, _ := rec.Events(today, today)
	if len(events) != 5 || events[1].Success {
		t.Errorf("Events = %+v", events)
	}

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	days, _ := rec.DailyCounts(yesterday, today)
	if len(days) != 2 || days[0] != 0 || days[1] != 4 {
		t.Errorf("DailyCounts = %v, want [0 4]", days)
	}
}
