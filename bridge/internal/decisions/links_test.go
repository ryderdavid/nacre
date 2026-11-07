package decisions

import (
	"io"
	"log"
	"reflect"
	"testing"
	"time"
)

func TestLinksByChatInvertsAndSorts(t *testing.T) {
	s, err := Open(t.TempDir(), time.Now, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	s.LinkWork("r_2", "c1")
	s.LinkWork("bead:nacre:NAC-1", "c1")
	s.LinkWork("r_9", "c2")
	got := s.LinksByChat()
	want := map[string][]string{"c1": {"bead:nacre:NAC-1", "r_2"}, "c2": {"r_9"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LinksByChat = %v, want %v", got, want)
	}
}
