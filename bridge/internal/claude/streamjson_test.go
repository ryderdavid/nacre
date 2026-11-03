package claude

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

func parse(t *testing.T, line string) Parsed {
	t.Helper()
	p, err := ParseLine([]byte(line))
	if err != nil {
		t.Fatalf("ParseLine(%s): %v", line, err)
	}
	return p
}

func TestParseInit(t *testing.T) {
	p := parse(t, `{"type":"system","subtype":"init","cwd":"/r","session_id":"abc","tools":["Bash"],"model":"claude-haiku-4-5","permissionMode":"plan"}`)
	if p.Init == nil || p.Init.SessionID != "abc" || p.Init.Model != "claude-haiku-4-5" || p.Init.CWD != "/r" {
		t.Fatalf("init = %+v", p.Init)
	}
	if len(p.Events) != 1 || p.Events[0].Kind != api.KindSystem || p.Events[0].Text != "Session started · model claude-haiku-4-5" {
		t.Fatalf("events = %+v", p.Events)
	}
}

func TestParseAssistantBlocks(t *testing.T) {
	line := `{"type":"assistant","message":{"id":"m1","role":"assistant","content":[
		{"type":"thinking","thinking":"Let me look."},
		{"type":"text","text":"Reading the bead."},
		{"type":"text","text":"   "},
		{"type":"tool_use","id":"tu1","name":"Bash","input":{"command":"bd show NAC-1\n--json","description":"Show"}},
		{"type":"tool_use","id":"tu2","name":"Read","input":{"file_path":"/r/main.go","limit":10}},
		{"type":"tool_use","id":"tu3","name":"Grep","input":{"pattern":"TODO","path":"/r"}},
		{"type":"tool_use","id":"tu4","name":"Task","input":{"prompt":"long","description":"Explore repo"}},
		{"type":"tool_use","id":"tu5","name":"mcp__x__y","input":{"n":1,"query":"first string","z":"later"}},
		{"type":"tool_use","id":"tu6","name":"ExitPlanMode"}
	]},"session_id":"abc"}`
	p := parse(t, strings.ReplaceAll(line, "\n\t\t", ""))
	kinds := []string{}
	for _, e := range p.Events {
		kinds = append(kinds, e.Kind)
	}
	wantKinds := []string{"thinking", "text", "tool_use", "tool_use", "tool_use", "tool_use", "tool_use", "tool_use"}
	if !reflect.DeepEqual(kinds, wantKinds) {
		t.Fatalf("kinds = %v", kinds)
	}
	summaries := []string{}
	for _, e := range p.Events[2:] {
		summaries = append(summaries, e.Text)
	}
	want := []string{"bd show NAC-1 --json", "/r/main.go", "TODO in /r", "Explore repo", "first string", ""}
	if !reflect.DeepEqual(summaries, want) {
		t.Fatalf("summaries = %q", summaries)
	}
	bash := p.Events[2]
	if bash.Tool != "Bash" || bash.ToolID != "tu1" {
		t.Fatalf("bash event = %+v", bash)
	}
	var in map[string]string
	if err := json.Unmarshal(bash.Input, &in); err != nil || in["description"] != "Show" {
		t.Fatalf("input not passed through raw: %s", bash.Input)
	}
	if string(p.Events[7].Input) != "{}" {
		t.Fatalf("missing input should become {}: %s", p.Events[7].Input)
	}
}

func TestParseToolResults(t *testing.T) {
	big := strings.Repeat("x", MaxToolResult+50)
	line := `{"type":"user","message":{"role":"user","content":[
		{"type":"tool_result","tool_use_id":"tu1","content":"ok output"},
		{"type":"tool_result","tool_use_id":"tu2","content":[{"type":"text","text":"a"},{"type":"image","source":{}},{"type":"text","text":"b"}],"is_error":true},
		{"type":"tool_result","tool_use_id":"tu3","content":"` + big + `"}
	]}}`
	p := parse(t, strings.ReplaceAll(line, "\n\t\t", ""))
	if len(p.Events) != 3 {
		t.Fatalf("events = %+v", p.Events)
	}
	e0, e1, e2 := p.Events[0], p.Events[1], p.Events[2]
	if e0.Kind != api.KindToolResult || e0.ToolID != "tu1" || e0.Text != "ok output" || e0.IsError == nil || *e0.IsError {
		t.Fatalf("e0 = %+v", e0)
	}
	if e1.Text != "a\n[image]\nb" || !*e1.IsError {
		t.Fatalf("e1 = %+v", e1)
	}
	if n := len([]rune(e2.Text)); n != MaxToolResult || !strings.HasSuffix(e2.Text, "…") {
		t.Fatalf("e2 not truncated: %d", n)
	}
}

func TestParseUserStringContentIgnored(t *testing.T) {
	p := parse(t, `{"type":"user","message":{"role":"user","content":"the prompt"}}`)
	if len(p.Events) != 0 {
		t.Fatalf("events = %+v", p.Events)
	}
}

func TestParseResult(t *testing.T) {
	p := parse(t, `{"type":"result","subtype":"success","is_error":false,"duration_ms":1234,"num_turns":3,"result":"Done.","session_id":"abc","total_cost_usd":0.0123}`)
	r := p.Result
	if r == nil || r.Text != "Done." || r.IsError || r.CostUSD != 0.0123 || r.DurationMS != 1234 || r.NumTurns != 3 || r.SessionID != "abc" {
		t.Fatalf("result = %+v", r)
	}
	if len(p.Events) != 1 || p.Events[0].Kind != api.KindResult || p.Events[0].Text != "Done." || *p.Events[0].IsError {
		t.Fatalf("events = %+v", p.Events)
	}

	p = parse(t, `{"type":"result","subtype":"error_max_turns","is_error":true,"session_id":"abc"}`)
	if p.Result.Text != "error_max_turns" || !*p.Events[0].IsError {
		t.Fatalf("error result = %+v", p.Result)
	}
}

func TestParseUnknownAndBlank(t *testing.T) {
	for _, line := range []string{
		``, `   `,
		`{"type":"stream_event","event":{}}`,
		`{"type":"rate_limit_event","rate_limit_info":{}}`,
		`{"type":"system","subtype":"hook_response","stdout":"x"}`,
		`{"type":"assistant","message":{"content":"string content"}}`,
		`{"type":"assistant"}`,
	} {
		p := parse(t, line)
		if len(p.Events) != 0 || p.Init != nil || p.Result != nil {
			t.Errorf("%q produced %+v", line, p)
		}
	}
	if _, err := ParseLine([]byte(`{not json`)); err == nil {
		t.Error("malformed JSON should error")
	}
}

func TestToolSummaryTruncation(t *testing.T) {
	s := ToolSummary("Bash", json.RawMessage(`{"command":"`+strings.Repeat("a", 500)+`"}`))
	if n := len([]rune(s)); n != maxSummary {
		t.Fatalf("len = %d", n)
	}
}

func TestTurnArgs(t *testing.T) {
	first := Turn{Prompt: "do it", Mode: "plan", Model: "haiku", Worktree: true, Name: "Nacre · NAC-1"}.Args()
	want := []string{"-p", "do it", "--output-format", "stream-json", "--verbose", "--permission-mode", "plan", "--model", "haiku", "--max-budget-usd", "50.00", "--worktree", "--name", "Nacre · NAC-1"}
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("first = %q", first)
	}
	follow := Turn{Prompt: "-continue", Mode: "auto", Effort: "xhigh", Resume: "sid", Worktree: true, Name: "ignored"}.Args()
	want = []string{"-p", " -continue", "--resume", "sid", "--output-format", "stream-json", "--verbose", "--permission-mode", "auto", "--effort", "xhigh", "--max-budget-usd", "50.00"}
	if !reflect.DeepEqual(follow, want) {
		t.Fatalf("follow = %q", follow)
	}
}

func TestPermissionDeniedBecomesSystemNotice(t *testing.T) {
	line := `{"type":"system","subtype":"permission_denied","tool_name":"Bash","message":"This command requires approval","session_id":"s"}`
	p, err := ParseLine([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Events) != 1 || p.Events[0].Kind != api.KindSystem || p.Events[0].Text != "Not allowed in this permission mode: This command requires approval" {
		t.Fatalf("got %+v", p.Events)
	}
}
