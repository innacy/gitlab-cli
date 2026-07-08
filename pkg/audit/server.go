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
	now := time.Now().UTC()
	rangeParam := r.URL.Query().Get("range")
	dateParam := r.URL.Query().Get("date")

	switch rangeParam {
	case "week":
		return now.AddDate(0, 0, -7).Format("2006-01-02"), now.Format("2006-01-02")
	case "month":
		if dateParam != "" {
			if parsed, err := time.Parse("2006-01", dateParam); err == nil {
				return parsed.Format("2006-01-02"), parsed.AddDate(0, 1, -1).Format("2006-01-02")
			}
		}
		return now.AddDate(0, 0, -30).Format("2006-01-02"), now.Format("2006-01-02")
	case "year":
		year := r.URL.Query().Get("year")
		if year == "" {
			year = fmt.Sprintf("%d", now.Year())
		}
		return year + "-01-01", year + "-12-31"
	default:
		return now.AddDate(0, 0, -30).Format("2006-01-02"), now.Format("2006-01-02")
	}
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

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(dashboardHTML)
}
