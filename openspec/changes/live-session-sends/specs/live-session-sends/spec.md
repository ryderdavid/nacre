## ADDED Requirements

### Requirement: Version guard with a read-only fallback
nacred SHALL offer a live send (`can_send_live: true`) only on an active adopted Claude Code run whose
session record has `kind` `interactive`, a `pidDomain` equal to this computer's, a non-empty
`messagingSocketPath` whose socket exists, `peerProtocol` exactly 1, and an `entrypoint` other than
`claude-desktop`. Before each send it SHALL also check that the pid is alive with the record's
`procStart` and that the socket accepts a connection. Any failed check, and any send error, SHALL answer
`POST /v1/runs/{run}/live` with 409 and the reason, write nothing to the session, and leave the run
read-only. `POST /v1/runs/{run}/followup` SHALL keep answering 409 for every adopted run.

#### Scenario: Unknown peer protocol
- **WHEN** an adopted session's record says `peerProtocol: 2`
- **THEN** the run has no `can_send_live`, `live_send_unavailable` names the protocol, and `POST .../live` is a 409

#### Scenario: Desktop session
- **WHEN** an adopted session's record says `entrypoint: claude-desktop`
- **THEN** no live send is offered and `POST .../live` is a 409

#### Scenario: Session died between polls
- **WHEN** the run offered a live send but the pid is gone (or reused) at send time
- **THEN** `POST .../live` is a 409, nothing is sent, and the run is unchanged

### Requirement: Sending a peer message
On a live send nacred SHALL bind its inbox at `<socket dir>/<nacred pid>.sock` with mode 0600 if it is
not bound yet, then write on one connection: an `auth` line with the session's `peerToken` when a key
for this process start exists; a `user` frame whose content is the message prefixed with
`<actor> (via Nacre): `, with `from` set to `uds:<inbox>` and a fresh UUID `msg_id`; and, when
`peerFeatures` contains `notify_idle`, a `notify_when_idle` control frame. The send SHALL be recorded in
the run's `peer_messages` with status `sent`, with a `user` event and a `system` event saying it arrives
as a peer message that cannot approve prompts or run slash commands.

#### Scenario: Frames on the wire
- **WHEN** Ryder sends "also run the tests" to a session that advertises `notify_idle`
- **THEN** the session's socket receives an auth line (if a key exists), a user frame with content "Ryder Cobean (via Nacre): also run the tests", and a notify_when_idle frame, all on one connection

### Requirement: Receipts and delivery confirmation
nacred SHALL apply each `peer_message_status` arriving at its inbox to the send it names (by
`orig_msg_id` and `dropped_msg_ids`), setting `status` and `reason` and adding a `system` event, marked
as an error for `denied`, `expired`, `refused` and `dropped`. It SHALL set `confirmed` only when the
session's transcript shows a user entry with `origin.kind` `peer` and `origin.msg_id` equal to the send's.
A send with neither a receipt nor confirmation within two minutes SHALL become `unconfirmed`, and nacred
SHALL NOT resend. A `peer_idle_notice` SHALL only set `idle_at`.

#### Scenario: Held then confirmed
- **WHEN** the session answers `held`, then the user approves it and the transcript gains the peer entry with our msg_id
- **THEN** the run shows the send as held, then as confirmed, with an event for each

#### Scenario: Refused
- **WHEN** the session answers `refused`
- **THEN** the send's status is `refused` and an error `system` event says so

### Requirement: The app's live-send action
The app SHALL show "Send to live session" only on a live adopted run with `can_send_live`, as an explicit
action that first states that the message arrives as a peer message, runs under the session's own
permissions, and cannot approve prompts or run slash commands. The run detail SHALL list the newest live
sends with their status, and show held, refused, dropped and unconfirmed sends to the user.

#### Scenario: Guard fails
- **WHEN** an adopted run has no `can_send_live`
- **THEN** the run detail shows the read-only note and no live-send action
