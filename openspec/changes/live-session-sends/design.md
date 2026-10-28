## Context

The wire protocol, observed behaviour and consent model are written up on NAC-j5z.12 (Claude Code
2.1.309 and 2.1.310). In short: one connection per send to the session's socket, NDJSON lines (optional
`auth`, a `user` frame with `from: uds:<inbox>` and a `msg_id`, optionally `notify_when_idle`), then
half-close. Replies arrive as new connections to the sender's inbox, which must be bound by the sending
process in the same socket directory and named `<pid>.sock`.

## Decisions

- **One explicit endpoint, not a changed follow-up.** `POST .../live` is separate from `.../followup`,
  so a client that knows nothing about it keeps getting the same 409, and the app can make it a
  deliberate action with its own copy.
- **Guard in two halves.** The record half (kind, entrypoint, pidDomain, socket path set and present,
  peerProtocol 1) is evaluated on every poll and drives `can_send_live`. Liveness with matching
  `procStart` and a 250 ms connect probe run only at send time, so nacred does not connect to every
  adopted session every ten seconds. Adoption itself already requires the pid be alive.
- **Inbox per socket directory, for nacred's lifetime.** A held message can be approved much later, so
  the inbox stays bound until shutdown; a stale socket file from an earlier process with the same pid is
  replaced only if nothing answers on it.
- **Confirmation from the transcript.** A `delivered` receipt says the session queued the message;
  `confirmed` is set only when the transcript shows the peer-origin user entry with our `msg_id`. With
  neither within two minutes the send is `unconfirmed`, and nacred never resends.
- **Auth line sent when available.** It is optional on macOS and Linux; nacred sends the `peerToken`
  from `~/.claude/sessions/<pid>.<hex>.key` only when that key's `procStart` matches the record.

## Risks

- The protocol is undocumented. A new Claude Code that changes it is expected to bump `peerProtocol`,
  which the guard refuses; a silent change shows up as `unconfirmed` sends, never as a resend.
- The turn runs under the session's own permission mode, so in auto mode a phone message can lead to
  tool use without a prompt — as it would typed at the desk. The action is explicit and says so.
