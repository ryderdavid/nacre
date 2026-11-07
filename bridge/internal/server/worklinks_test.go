package server

import (
	"reflect"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/chat"
)

func TestWorkLinksParsesBeadsChangesAndWork(t *testing.T) {
	got := workLinks([]string{"bead:nacre:NAC-1.2", "change:nacre:ch_1", "r_abc", "bead:x"})
	want := []chat.WorkLink{
		{Kind: "bead", Repo: "nacre", ID: "NAC-1.2"},
		{Kind: "change", Repo: "nacre", ID: "ch_1"},
		{Kind: "work", ID: "r_abc"},
		{Kind: "work", ID: "bead:x"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("workLinks = %+v, want %+v", got, want)
	}
	if workLinks(nil) != nil {
		t.Fatal("no keys should give no links")
	}
}
