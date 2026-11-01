package version

import "testing"

func TestResolve(t *testing.T) {
	for _, c := range []struct {
		version, commit string
		stamp           vcs
		wantV, wantC    string
	}{
		{"0.6.0", "abc", vcs{revision: "def", modified: true}, "0.6.0", "abc"},
		{"0.6.0-3-g1234567-dirty", "", vcs{revision: "def"}, "0.6.0-3-g1234567-dirty", "def"},
		{"", "", vcs{revision: "def"}, "dev", "def"},
		{"", "", vcs{revision: "def", modified: true}, "dev-dirty", "def"},
		{"", "", vcs{}, "dev", ""},
	} {
		v, cm := resolve(c.version, c.commit, c.stamp)
		if v != c.wantV || cm != c.wantC {
			t.Errorf("resolve(%q, %q, %+v) = %q, %q; want %q, %q", c.version, c.commit, c.stamp, v, cm, c.wantV, c.wantC)
		}
	}
}
