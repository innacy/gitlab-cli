package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPISummary(t *testing.T) {
	rec, _ := OpenInMemory()
	defer rec.Close()
	seedEvents(t, rec)

	srv := NewServer(rec)
	req := httptest.NewRequest("GET", "/api/summary?range=month", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["total_events"].(float64) != 5 {
		t.Errorf("total_events=%v want 5", result["total_events"])
	}
}

func TestAPIHeatmap(t *testing.T) {
	rec, _ := OpenInMemory()
	defer rec.Close()
	seedEvents(t, rec)

	srv := NewServer(rec)
	year := fmt.Sprintf("%d", time.Now().Year())
	req := httptest.NewRequest("GET", "/api/heatmap?year="+year, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
}

func TestAPIDashboardHTML(t *testing.T) {
	rec, _ := OpenInMemory()
	defer rec.Close()

	srv := NewServer(rec)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("content-type=%q want text/html", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("empty dashboard response")
	}
}

func TestDashboardIsSelfContained(t *testing.T) {
	if bytes.Contains(dashboardHTML, []byte("<script src")) || bytes.Contains(dashboardHTML, []byte("cdn.")) {
		t.Error("dashboard loads external scripts; it must work offline")
	}
	for _, id := range []string{`id="commands"`, `id="projects"`, `value="today"`, `value="year"`, `id="error"`, "@media (max-width", "function esc("} {
		if !bytes.Contains(dashboardHTML, []byte(id)) {
			t.Errorf("dashboard missing %s", id)
		}
	}
	rec, _ := OpenInMemory()
	defer rec.Close()
	w := httptest.NewRecorder()
	NewServer(rec).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("CSP = %q", csp)
	}
	w = httptest.NewRecorder()
	NewServer(rec).ServeHTTP(w, httptest.NewRequest("GET", "/favicon.ico", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown path status = %d, want 404", w.Code)
	}
}

func TestAPICommandsAndProjects(t *testing.T) {
	rec, _ := OpenInMemory()
	defer rec.Close()
	id, _ := rec.Record(Event{Command: "mr-approve", Category: CategoryGitLabMutation, Project: "<img src=x>", Success: true})
	rec.Finish(id, 10, false)
	srv := NewServer(rec)

	var cmds []CommandRow
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("GET", "/api/commands?range=today", nil))
	json.Unmarshal(w.Body.Bytes(), &cmds)
	if len(cmds) != 1 || cmds[0].Command != "mr-approve" || cmds[0].Failed != 1 {
		t.Errorf("/api/commands = %s", w.Body.String())
	}

	var projects []ProjectRow
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("GET", "/api/projects?range=year", nil))
	json.Unmarshal(w.Body.Bytes(), &projects)
	if len(projects) != 1 || projects[0].Project != "<img src=x>" {
		t.Errorf("/api/projects = %s", w.Body.String())
	}
}
