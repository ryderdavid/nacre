# Host permission prompt recordings

`host_prompt_deny.jsonl` and `host_prompt_allow.jsonl` were recorded on
2026-11-05 from the installed Claude Code 2.1.318 (`claude --version`). Each
line is `{"dir": "stdin" | "stdout", "line": <the JSON line>}`, in order.

How they were made: in a scratch git repository with a local bare remote, the
CLI ran as

```
claude -p --input-format stream-json --output-format stream-json --verbose \
  --permission-mode auto --permission-prompts host --permission-prompt-tool stdio \
  --max-budget-usd 2 --no-session-persistence \
  --settings '{"permissions":{"ask":["Bash(git push:*)"]}}'
```

and was asked to run `git push --force origin HEAD:main`. The host answered
the `can_use_tool` control request with a deny carrying the message
`use the debug build` (deny recording), or with an allow repeating the input as
`updatedInput` (allow recording; the push ran against the scratch remote).

What the recordings settle:

- **`--permission-prompt-tool stdio` is required.** With `--permission-prompts
  host` alone, the CLI never sent a control request: it refused the step itself
  (a `permission_denied` system line and the tool result "Claude requested
  permissions to use Bash, but you haven't granted it yet."). With `stdio` it
  sends `{"type":"control_request","request_id":…,"request":{"subtype":"can_use_tool",…}}`
  on stdout and waits for a `control_response` on stdin. `claude.Chat` passes
  both flags.
- **The request** carries `tool_name`, `display_name`, `input`, `description`,
  `tool_use_id`, `decision_reason_type` and, for a rule, `decision_reason_rule_source`.
  This one has no free-text reason; a classifier reason, when the CLI gives one,
  is read from `decision_reason` or `reason`.
- **The answer** is `{"type":"control_response","response":{"subtype":"success","request_id":…,"response":{"behavior":"deny","message":…}}}`
  or `…{"behavior":"allow","updatedInput":{…}}`. A deny's message comes back to
  the agent as the tool result, and the turn continues to its result line.
- **Why an `ask` rule.** The auto classifier on this computer let every test
  command run without asking (a force push to a local remote, `rm -rf` of a
  missing path, `curl … | sh` of a 404), so the prompt was forced with an `ask`
  rule. The control messages are the same whatever made the CLI ask.

Trimmed for size and privacy: hook, skills-list, rate-limit and thinking lines,
and the long fields of the init and result lines. The control request and
response lines are verbatim.

`internal/chat/fakeclaude` replays these files as the claude binary in Go tests.

# Light plan-mode turn recordings (routing, section 4.4)

`light_plain.jsonl`, `light_readonly_bash.jsonl`, `light_handoff_line.jsonl`
and `light_exit_plan.jsonl` were recorded on 2026-11-05 from the installed
Claude Code 2.1.318, in a scratch git repository with one commit, with the
light turn's argv:

```
claude -p --input-format stream-json --output-format stream-json --verbose \
  --permission-mode plan --permission-prompts host --permission-prompt-tool stdio \
  --model sonnet --append-system-prompt <briefing> --max-budget-usd 1 \
  --no-session-persistence
```

(`--no-session-persistence` only kept the scratch sessions off disk; nacred's
light turns use `--session-id`/`--resume` like full ones.) The user message was
the "On screen" note with the light addendum, then the question. The same
trimming as above applies.

- **Plain answer** (`light_plain`): "In one sentence: what is the difference
  between a git merge and a rebase?" One text block and a result. No tool, no
  host prompt.
- **Read-only Bash step** (`light_readonly_bash`): "Run git log --oneline -1
  and git status --short …". **The step ran without asking**: plan mode ran
  the Bash call (`git log --oneline -1; …; git status --short`) on its own, with
  no `can_use_tool` control request, and answered from its output. So read
  steps stay inside the light turn; they don't hand off.
- **Handoff line** (`light_handoff_line`): asked to add a line to README.md,
  with the addendum in the note, the light model answered with its plan and
  ended with `NACRE_HANDOFF: Appending 'Built by Nacre.' to README.md needs a
  file edit, which this read-only chat can't do.` It called no tool.
- **`ExitPlanMode` attempt** (`light_exit_plan`): the same request without the
  addendum, asking it to call `ExitPlanMode`. Plan mode first wrote its plan to
  `~/.claude/plans/<name>.md` without asking (the plan file is the one write
  plan mode allows), loaded `ExitPlanMode` with `ToolSearch`, then called it.
  **`ExitPlanMode` raised a host prompt**: a `can_use_tool` control request with
  `tool_name: "ExitPlanMode"` and the plan in `input.plan`. The host (as nacred
  does) denied it with "Handing this to the full harness" and then sent
  `{"type":"control_request","request_id":…,"request":{"subtype":"interrupt"}}`.
  The CLI answered the interrupt with a `control_response`, then a result line
  with `subtype: "error_during_execution"` that still carries `total_cost_usd`,
  so the stopped light turn's cost is counted. Without the interrupt (an
  earlier take) the model carried on after the deny and explained it couldn't
  edit, which nacred would not want shown.

`light_model_rejected.jsonl` is not a plan-mode recording: it is the shape the
same CLI printed for `--model bogus-model-x` (an assistant text with
`"error":"model_not_found"` and an `is_error` result: "There's an issue with
the selected model (…). It may not exist or you may not have access to it."),
with the model name changed. After such a call the session exists and holds
the message, and `--resume` on it works (checked by hand the same day).

The routing call itself was checked by hand the same day: `--model haiku`,
`--safe-mode --permission-prompts none --no-session-persistence --tools ""`,
`--max-budget-usd 0.10` and the `{route, reason}` schema answered
`structured_output: {"route":"light", …}` for "what is 2+2?" for $0.0003.

`internal/chat/fakeclaude` replays the light recordings for light turns (see its
package comment), and `route_test.go` checks each case.
