package server

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/daemonupdate"
)

func updateServer(t *testing.T) *Server {
	return &Server{Config: &config.Config{Token: "fixture"}, Log: log.New(io.Discard, "", 0), Updates: daemonupdate.New(daemonupdate.Options{Dir: t.TempDir(), Launch: func(daemonupdate.Status) error { return nil }})}
}
func updateRequest(s *Server, method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestDaemonUpdateAuthInputAndMaintenance(t *testing.T) {
	s := updateServer(t)
	for _, tc := range []struct {
		method, path, body, token string
		want                      int
	}{
		{"POST", "/v1/daemon/update", "{}", "", 401}, {"GET", "/v1/daemon/update", "", "", 401},
		{"POST", "/v1/daemon/update", `{"command":"arbitrary"}`, "fixture", 400},
		{"POST", "/v1/daemon/update", `{} {}`, "fixture", 400},
		{"POST", "/v1/daemon/update", `{}`, "fixture", 202},
		{"POST", "/v1/daemon/update", `{}`, "fixture", 202},
		{"POST", "/v1/repos/anything/rounds", `{}`, "fixture", 503},
		{"GET", "/v1/daemon/update", "", "fixture", 200},
	} {
		w := updateRequest(s, tc.method, tc.path, tc.body, tc.token)
		if w.Code != tc.want {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}
func TestDaemonUpdateCannotRaceMutation(t *testing.T) {
	s := updateServer(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	handler := s.maintenance(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { close(entered); <-release }))
	go func() {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/work", nil))
		close(done)
	}()
	<-entered
	w := updateRequest(s, "POST", "/v1/daemon/update", "{}", "fixture")
	if w.Code != 409 || s.Updates.Get().Active {
		t.Fatal(w.Code, w.Body.String())
	}
	close(release)
	<-done
	w = updateRequest(s, "POST", "/v1/daemon/update", "{}", "fixture")
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
}
