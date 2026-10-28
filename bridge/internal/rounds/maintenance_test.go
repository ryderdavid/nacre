package rounds

import (
	"context"
	"github.com/ryderdavid/nacre/bridge/internal/api"
	"testing"
)

func TestDaemonUpdateWaitsForTerminalRoundCleanup(t *testing.T) {
	for _, state := range []string{api.RoundLanded, api.RoundCancelled} {
		for _, kind := range []string{"operation", "side-effect"} {
			t.Run(state+"/"+kind, func(t *testing.T) {
				m := &Manager{ctx: context.Background(), rounds: map[string]*round{}}
				r := &round{m: m, dir: t.TempDir(), meta: api.Round{ID: "round", State: state}, ops: map[string]bool{}, kick: make(chan struct{}, 1)}
				m.rounds["round"] = r
				if m.Busy() {
					t.Fatal("settled terminal round busy")
				}
				entered := make(chan struct{})
				release := make(chan struct{})
				r.mu.Lock()
				if kind == "operation" {
					r.goOp("close", func(context.Context) func() { close(entered); <-release; return nil })
				} else {
					r.async(func(context.Context) { close(entered); <-release })
				}
				r.mu.Unlock()
				<-entered
				if !m.Busy() {
					t.Fatal("terminal cleanup did not reserve maintenance")
				}
				close(release)
				m.wg.Wait()
				if m.Busy() {
					t.Fatal("completed cleanup still busy")
				}
			})
		}
	}
}
