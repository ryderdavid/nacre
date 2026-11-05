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
