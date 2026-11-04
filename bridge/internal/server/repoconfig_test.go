package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const draftTranscript = `{"type":"system","subtype":"init","session_id":"s-1"}
{"type":"result","subtype":"success","is_error":false,"session_id":"s-1","total_cost_usd":0.12,"result":"","structured_output":{"yaml":"gates:\n  - name: tests\n    run: go test ./...\n","notes":"Runs the Go tests."}}`

func TestDraftRepoConfigValidatesTheDraft(t *testing.T) {
	ts, repoID := assistServer(t, draftTranscript)
	resp, body := post(t, ts.URL+"/v1/repos/"+repoID+"/config/draft", `{}`)
	if resp.StatusCode != 200 || body["problem"] != nil || !strings.Contains(body["yaml"].(string), "go test ./...") || body["notes"] != "Runs the Go tests." {
		t.Fatalf("status %d: %v", resp.StatusCode, body)
	}

	// A draft nacred can't use comes back with the reason, for Ryder to fix.
	bad := strings.Replace(draftTranscript, `run: go test ./...\n`, `\n`, 1)
	ts, repoID = assistServer(t, bad)
	if resp, body = post(t, ts.URL+"/v1/repos/"+repoID+"/config/draft", `{}`); resp.StatusCode != 200 || body["problem"] == nil {
		t.Fatalf("unusable draft: %d %v", resp.StatusCode, body)
	}
}

func TestSaveRepoConfigCommitsOnlyNacreYAML(t *testing.T) {
	ts, repoID, dir := assistServerIn(t, "#!/bin/sh\ncat <<'EOF'\n"+draftTranscript+"\nEOF\n")
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "work")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	git("add", "a.txt")
	git("commit", "-q", "-m", "first")
	// Ryder's own staged edit must stay out of the commit.
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("edited"), 0o644)
	git("add", "a.txt")
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")

	if resp, _ := post(t, ts.URL+"/v1/repos/"+repoID+"/config", `{"yaml":"gates: []\n"}`); resp.StatusCode != 400 {
		t.Fatalf("invalid file accepted: %d", resp.StatusCode)
	}
	resp, body := post(t, ts.URL+"/v1/repos/"+repoID+"/config", `{"yaml":"gates:\n  - name: tests\n    run: go test ./...\n"}`)
	if resp.StatusCode != 200 || body["branch"] != "work" {
		t.Fatalf("status %d: %v", resp.StatusCode, body)
	}
	if files := git("show", "--name-only", "--format=", "HEAD"); files != "nacre.yaml" {
		t.Fatalf("commit holds %q", files)
	}
	if staged := git("diff", "--cached", "--name-only"); staged != "a.txt" {
		t.Fatalf("staged edit lost: %q", staged)
	}
	// It never overwrites, and drafting stops once the file exists.
	if resp, _ = post(t, ts.URL+"/v1/repos/"+repoID+"/config", `{"yaml":"gates:\n  - name: x\n    run: true\n"}`); resp.StatusCode != 409 {
		t.Fatalf("overwrite: %d", resp.StatusCode)
	}
	if resp, _ = post(t, ts.URL+"/v1/repos/"+repoID+"/config/draft", `{}`); resp.StatusCode != 409 {
		t.Fatalf("draft over existing: %d", resp.StatusCode)
	}
}
