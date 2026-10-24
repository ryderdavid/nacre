package bd

import (
	"strings"
	"testing"
)

func TestAddAndRemoveLabels(t *testing.T) {
	f := strings.Join(UpdateParams{AddLabels: []string{"round:abc"}, RemoveLabels: []string{"round:old"}}.flags(), " ")
	if !strings.Contains(f, "--add-label=round:abc") || !strings.Contains(f, "--remove-label=round:old") {
		t.Errorf("flags = %s", f)
	}
	if !(UpdateParams{AddLabels: []string{"x"}}).Empty() == false {
		t.Error("AddLabels alone counts as empty")
	}
}
