# claude testdata

`transcript.jsonl` and `transcript_sidechain.jsonl` are fixtures for the on-disk
session-transcript adapter (`transcript.go`).

`transcript.jsonl` is real lines captured from `~/.claude/projects/*/*.jsonl` on
a development Mac (Claude Code 2.1.263 and 2.1.304), one per entry type the
adapter handles plus one per entry type it must ignore. The lines are kept in
their real shape; only the contents were scrubbed:

- `sessionId` is rewritten to one fixed id, every other UUID to a stable fake;
- `/Users/ryder` → `/Users/dev`, the owner's name, email, Tailscale address and
  reverse-domain package prefix replaced;
- base64 image and blob payloads replaced with `BASE64`;
- long strings truncated to 400 characters (120 for `api-request-shape`, whose
  `shape` also keeps only the first system block and two tool schemas).

The entries come from several sessions, so timestamps are not monotonic across
the file. The adapter does not assume they are.

`transcript_sidechain.jsonl` is three of those same real lines with
`isSidechain` flipped to `true`. No session in the captured corpus had a
sidechain entry (0 of 15,615), so the flag itself is synthetic while the entry
shape is real.

To refresh either fixture, re-capture and re-scrub; do not hand-edit entries
into shapes Claude Code does not actually write.
