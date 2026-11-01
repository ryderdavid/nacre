## Why

Adopted runs are read-only: a follow-up is refused (409) because `claude -p --resume` against a live
interactive session forks it. Spike NAC-j5z.12 showed that Claude Code's own session-to-session channel
(the record's `messagingSocketPath`, peerProtocol 1) lets another process queue a turn in a live session
without forking it, with receipts and an idle notice, and that the receiving session labels such a turn
as a peer message that cannot approve prompts or run slash commands. That makes "tell the agent at my
desk one more thing" possible from the phone, safely, for sessions that look exactly like the ones tested.

## What Changes

- nacred reads `pidDomain`, `messagingSocketPath`, `peerProtocol` and `peerFeatures` from session records.
- A new peer-messaging client in nacred: the version guard, NDJSON frame encoding, an inbox socket at
  `<socket dir>/<nacred pid>.sock` (0600) for `peer_message_status` and `peer_idle_notice`, and the
  optional auth line from the session's key file.
- `POST /v1/runs/{run}/live` hands an adopted run's session a message, prefixed with who sent it. Any
  guard failure or send error is a 409 and the run stays read-only; `POST .../followup` is unchanged.
- Run gains `can_send_live`, `live_send_unavailable` and `peer_messages` (status from receipts,
  `confirmed` from the transcript's `origin.msg_id`).
- App: "Send to live session" on adopted runs that pass the guard, as an explicit two-step action whose
  copy says it arrives as a peer message that cannot approve prompts or run slash commands; each send's
  status (held, refused, dropped, unconfirmed, delivered) is shown on the run.

Out of scope:
- Sessions hosted by the Claude desktop app (`entrypoint: claude-desktop`), untested in the spike.
- Any peerProtocol other than 1, remote or bridged sessions, Codex sessions.
- `priority: now` (interrupting a turn), file attachments, artifact replies, renaming sessions.
- Registering nacred in `~/.claude/sessions` so its idle-notice requests carry a detail line.
- Resending automatically, or keeping receipts across a nacred restart.

## Capabilities

### New Capabilities
- `live-session-sends`: sending a follow-up into a live adopted Claude Code session over its messaging
  socket, behind a version guard with the read-only fallback, and reporting what became of it.

### Modified Capabilities
<!-- None in openspec/specs/: adopted runs predate OpenSpec in this repository. -->

## Impact

- **nacred**: `internal/peer` (new), `internal/sessions` (record fields, `Alive`), `internal/claude`
  (peer-origin transcript entries), `internal/runs` (`SendLive`, `PeerFrame`, guard folded into each
  poll), `internal/server` (`POST /v1/runs/{run}/live`), `cmd/nacred` wiring.
- **API** (`docs/API.md`): the new endpoint and Run fields, under "Live sends".
- **App**: run detail controls and the live-message list; `BridgeClient.sendLive`.
