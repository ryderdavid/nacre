Gates used below: **G-bridge** = `cd bridge && go vet ./... && go test -race ./...`;
**G-app** = `cd android && ./gradlew -q testDebugUnitTest`; **G-specs** = `openspec validate --all --strict`.

## 1. nacred

- [x] 1.1 Session record peer fields, `Record.Peer()`, `sessions.Alive`; peer-origin `msg_id`s from the transcript — G-bridge
- [x] 1.2 `internal/peer`: guard (`CheckRecord`, `Client.Check`), frame encoding/decoding, inbox, key file, `Send` — G-bridge (guard table: unknown protocol, dead pid, missing socket, desktop entrypoint; frame golden; round trip against a fake session)
- [x] 1.3 Runs: `can_send_live` folded on each poll, `SendLive`, `PeerFrame`, transcript confirmation, unconfirmed window — G-bridge
- [x] 1.4 `POST /v1/runs/{run}/live`, nacred wiring, `docs/API.md` "Live sends" — G-bridge

## 2. App

- [x] 2.1 Run fields, `PeerMessage` labels, `BridgeClient.sendLive` — G-app
- [x] 2.2 Run detail: "Send to live session" with the peer-message note; live-message list — G-app

## 3. By hand

- [x] 3.1 Send to a real interactive session on the installed Claude Code and record the version on NAC-j5z.15
  (2.1.312, 2026-11-01: `Client.Check` passed, the send arrived as a peer message and was answered,
  `peer_idle_notice` came back for our notify id, and the transcript line had `origin.kind=peer` with our `msg_id`)
- [ ] 3.2 In the app: the action appears only on guarded runs; held, refused and dropped sends show
