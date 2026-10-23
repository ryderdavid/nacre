package repos

import "testing"

func TestPrefix(t *testing.T) {
	cases := []struct {
		ids  []string
		want string
	}{
		{nil, ""},
		{[]string{"GanttGen-eed.4", "GanttGen-x1", "GanttGen-eed"}, "GanttGen"},
		{[]string{"gvmcp-7-6", "gvmcp-6-1", "gvmcp-7-1"}, "gvmcp"},
		{[]string{"NAC-013.7", "NAC-013"}, "NAC"},
		{[]string{"hermes-android-x1y"}, "hermes-android"},
		{[]string{"NAC-013"}, "NAC"},
		{[]string{"personal-7jr.2", "personal-abc"}, "personal"},
		{[]string{"noprefix"}, ""},
	}
	for _, c := range cases {
		if got := Prefix(c.ids); got != c.want {
			t.Errorf("Prefix(%v) = %q, want %q", c.ids, got, c.want)
		}
	}
}
