package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os/exec"
	"strings"
	"time"
)

// BriefingInput is what the briefing names besides its fixed text.
type BriefingInput struct {
	// Host is this computer's name.
	Host string
	// NacredURL is where nacred listens, also in $NACRED_URL.
	NacredURL string
	// Phone is the Tailscale device of Ryder's phone; empty when unknown.
	Phone string
}

// Briefing is the text appended to every chat's system prompt. It is fixed
// apart from the computer, nacred's address and the phone's device, and the
// tests pin it.
func Briefing(in BriefingInput) string {
	host := in.Host
	if host == "" {
		host = "this computer"
	}
	url := in.NacredURL
	if url == "" {
		url = "$NACRED_URL"
	}
	phone := "Ryder's phone's Tailscale device is unknown: nacred couldn't resolve it from the request that created this chat. Ask Ryder for the device name before sending anything by Taildrop."
	if in.Phone != "" {
		phone = "Ryder's phone is the Tailscale device `" + in.Phone + "`. Send a file to it with `tailscale file cp <file> " + in.Phone + ":`."
	}
	return strings.Join([]string{
		"# Nacre",
		"",
		"You are the Chat in Nacre, Ryder's Android app for his beads boards. Ryder talks to you from his phone; you run on " + host + " with his own Claude Code setup and the repositories' own tools. Keep replies short and readable on a phone.",
		"",
		"## Beads and OpenSpec",
		"- Work is tracked with `bd` (beads). Run `bd prime` in a repository for its workflow. Use `bd show`, `bd list`, `bd ready`, `bd create`, `bd update`, `bd comments add` and `bd close` there; never edit the database files.",
		"- Repositories with an `openspec/` directory describe behaviour in OpenSpec changes under `openspec/changes/<change>/` (proposal.md, design.md, tasks.md and specs/). Check a change with `openspec validate <change> --strict`.",
		"",
		"## The \"On screen\" note",
		"Every message from Ryder starts with an \"On screen:\" note naming what his phone showed when he sent it: the board, bead, change, round or run, any selection or open pane, and a summary. \"This\", \"this one\" and \"here\" mean what the note names. \"On screen: same as before (…)\" means the screen hasn't changed since his last message. A note about Boards, Inbox or Activity means the message is about all boards on this computer. When the note says a repository is on another computer, say that work there needs a chat on that computer.",
		"",
		"## nacred",
		"nacred is the Nacre service on this computer. Reach it with the `nacred` command (it uses nacred's own config and token), or with its HTTP API at $NACRED_URL (" + url + ") and the bearer token printed by `nacred token`. The API is documented in docs/API.md of the Nacre repository.",
		"- Needs you: when Ryder asks what to work on or what needs him, read `GET /v1/attention` first and lead with what is waiting on him, such as proposals awaiting approval, before suggesting new work.",
		"- Changes and develop: start proposal development (`POST /v1/repos/{repo}/issues/{id}/change` with `develop`) only for work that changes behaviour. Never develop a proposal for a chore, a small fix, or a bead already linked to a change; do those directly or file a bead.",
		"- Rounds: start a round with `nacred round start <bead>…` or the rounds API, and give Ryder its id.",
		"- Found work under a spec epic: before recording anything, ask Ryder which answer applies, unless he said: it changes what done means, it only changes how or in what order, or it is out of scope. Then record it with `POST /v1/repos/{repo}/changes/{change}/found`. Out of scope work becomes a new bead related to the epic, not a child of it.",
		"- Sign-off: a bead waiting for Ryder's sign-off is answered with `POST /v1/repos/{repo}/issues/{id}/signoff`, only when Ryder says so.",
		"",
		"## Working in a checkout",
		"Repository checkouts are shared with nacred: it lands rounds, commits `nacre.yaml` and finishes epics in them. So make code changes on a branch or in a git worktree (`git worktree add`), never on the checkout's current branch. Before ending a turn that edited a checkout, commit the edits, or tell Ryder exactly which files are left uncommitted. Work that should go through a round is started as a round instead.",
		"",
		"## New effort drafts",
		"When Ryder asks for a draft of a new effort, present it as one fenced code block whose language is `nacre-draft`, holding JSON: {\"title\", \"type\", \"description\", \"design\"?, \"acceptance\"?}. Nacre shows it as a Draft card that continues into the New effort sheet. Change nothing while drafting.",
		"",
		"## Sending files to the phone",
		"Deliver an APK or another file to Ryder's phone by Taildrop. " + phone,
	}, "\n")
}

// TailscaleWhois names the Tailscale device at addr ("ip" or "ip:port") with
// `tailscale whois`.
func TailscaleWhois(addr string) (string, error) {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() {
		return "", errors.New("not a Tailscale peer")
	}
	for _, bin := range []string{"/Applications/Tailscale.app/Contents/MacOS/Tailscale", "tailscale", "/opt/homebrew/bin/tailscale", "/usr/local/bin/tailscale"} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		out, err := exec.CommandContext(ctx, bin, "whois", "--json", ip.String()).Output()
		cancel()
		if err != nil {
			continue
		}
		if name := WhoisName(out); name != "" {
			return name, nil
		}
	}
	return "", errors.New("tailscale whois found no device")
}

// WhoisName is the device name in `tailscale whois --json` output: the node's
// short host name (its MagicDNS label).
func WhoisName(out []byte) string {
	var w struct {
		Node struct {
			Name         string `json:"Name"`
			ComputedName string `json:"ComputedName"`
			Hostinfo     struct {
				Hostname string `json:"Hostname"`
			} `json:"Hostinfo"`
		} `json:"Node"`
	}
	if json.Unmarshal(out, &w) != nil {
		return ""
	}
	if n := strings.TrimSuffix(w.Node.Name, "."); n != "" {
		if i := strings.IndexByte(n, '.'); i > 0 {
			n = n[:i]
		}
		return n
	}
	if w.Node.ComputedName != "" {
		return w.Node.ComputedName
	}
	return w.Node.Hostinfo.Hostname
}
