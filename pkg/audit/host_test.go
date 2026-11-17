package audit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalOnlyRejectsForeignHost(t *testing.T) {
	h := localOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	for host, want := range map[string]int{
		"127.0.0.1:5000": 200, "localhost:5000": 200, "[::1]:5000": 200,
		"evil.example:5000": 403, "evil.example": 403, "127.0.0.1.evil.example": 403,
	} {
		req := httptest.NewRequest("GET", "/api/summary", nil)
		req.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != want {
			t.Errorf("Host %q: status %d, want %d", host, w.Code, want)
		}
	}
}
