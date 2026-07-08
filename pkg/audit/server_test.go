package audit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
