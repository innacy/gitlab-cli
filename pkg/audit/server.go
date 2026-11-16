package audit

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

//go:embed dashboard/index.html
var dashboardHTML []byte

type Server struct {
	mux      *http.ServeMux
	recorder *Recorder
}

func NewServer(rec *Recorder) *Server {
	s := &Server{
		mux:      http.NewServeMux(),
		recorder: rec,
	}
	s.mux.HandleFunc("/api/summary", s.handleSummary)
	s.mux.HandleFunc("/api/heatmap", s.handleHeatmap)
	s.mux.HandleFunc("/api/accuracy", s.handleAccuracy)
	s.mux.HandleFunc("/api/models", s.handleModels)
	s.mux.HandleFunc("/api/model-stats", s.handleModelStats)
	s.mux.HandleFunc("/api/model-performance", s.handleModelPerformance)
	s.mux.HandleFunc("/api/commands", s.handleCommands)
	s.mux.HandleFunc("/api/projects", s.handleProjects)
	s.mux.HandleFunc("/", s.handleDashboard)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) Start(openBrowser func(string)) (addr string, shutdown func(), err error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("listen: %w", err)
	}

	addr = fmt.Sprintf("http://%s", listener.Addr().String())

	srv := &http.Server{
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  30 * time.Minute,
	}

	go srv.Serve(listener)

	go func() {
		<-time.After(30 * time.Minute)
		srv.Shutdown(context.Background())
	}()

	if openBrowser != nil {
		openBrowser(addr)
	}

	return addr, func() { srv.Shutdown(context.Background()) }, nil
}

func (s *Server) parseDateRange(r *http.Request) (string, string) {
	q := r.URL.Query()
	from, to, _, err := DateRange(q.Get("range"), q.Get("date"), time.Now())
	if err != nil {
		from, to, _, _ = DateRange("month", "", time.Now())
	}
	return from, to
}

func (s *Server) writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseDateRange(r)
	summary, err := s.recorder.Summary(from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, map[string]interface{}{
		"total_events":    summary.TotalEvents,
		"ai_generations":  summary.AIGenerations,
		"mrs_created":     summary.MRsCreated,
		"mrs_reviewed":    summary.MRsReviewed,
		"tickets_created": summary.TicketsCreated,
		"failed":          summary.Failed,
		"reviews_posted":  summary.ReviewsPosted,
		"files_written":   summary.FilesWritten,
		"from":            from,
		"to":              to,
	})
}

func (s *Server) handleHeatmap(w http.ResponseWriter, r *http.Request) {
	year := r.URL.Query().Get("year")
	if year == "" {
		year = fmt.Sprintf("%d", time.Now().Year())
	}
	from := year + "-01-01"
	to := year + "-12-31"

	days, err := s.recorder.Heatmap(from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, days)
}

func (s *Server) handleAccuracy(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseDateRange(r)
	acc, err := s.recorder.AccuracyStats(from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, map[string]interface{}{
		"accepted":       acc.Accepted,
		"edited":         acc.Edited,
		"discarded":      acc.Discarded,
		"avg_edit_ratio": acc.AvgEditRatio,
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseDateRange(r)
	models, err := s.recorder.ModelUsage(from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, models)
}

func (s *Server) handleModelStats(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseDateRange(r)
	stats, err := s.recorder.ModelUsageByDate(from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, stats)
}

func (s *Server) handleModelPerformance(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseDateRange(r)
	perf, err := s.recorder.ModelPerformance(from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, perf)
}

func (s *Server) handleCommands(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseDateRange(r)
	rows, err := s.recorder.CommandBreakdown(from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, rows)
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	from, to := s.parseDateRange(r)
	rows, err := s.recorder.ProjectBreakdown(from, to, 10)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, rows)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The page is self-contained; forbid anything external.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'")
	w.Write(dashboardHTML)
}
