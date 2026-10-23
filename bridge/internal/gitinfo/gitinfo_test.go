package gitinfo

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"git@github.com:RyderDavid/Nacre.git":             "github.com/ryderdavid/nacre",
		"https://github.com/ryderdavid/nacre":             "github.com/ryderdavid/nacre",
		"https://user:pw@github.com/ryderdavid/nacre.git": "github.com/ryderdavid/nacre",
		"ssh://git@gitlab.example.com:2222/team/app.git":  "gitlab.example.com/team/app",
		"git@ssh.dev.azure.com:v3/org/proj/repo":          "ssh.dev.azure.com/v3/org/proj/repo",
		"/Users/ryder/src/repo":                           "",
		"file:///tmp/repo.git":                            "",
		"../sibling":                                      "",
		"":                                                "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadNonRepo(t *testing.T) {
	if got := Read(t.Context(), t.TempDir()); got != (Info{}) {
		t.Errorf("Read(non-repo) = %+v, want empty", got)
	}
}

func TestResolveAliasKeepsHTTPSRemotes(t *testing.T) {
	if got := resolveAlias(t.Context(), "github.com/a/b", "https://github.com/a/b.git"); got != "github.com/a/b" {
		t.Errorf("got %q", got)
	}
}
