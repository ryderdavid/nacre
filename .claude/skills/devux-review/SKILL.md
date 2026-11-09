---
name: devux-review
description: Agentic developer-UX review of Nacre. Runs lens reviewers in parallel (benchmark against the Claude Code and Codex apps, persona journeys, convergence and sign-off, confusion recovery, visual thinking, and a second-harness read from Codex) on one phase of the work, then merges, verifies and ranks their findings into a report under docs/devux/. Use when Ryder asks what a phase of Nacre needs, how it compares with other harnesses, or for a UX review of a flow.
---

# Developer UX review

Nacre is Ryder's harness for beads boards and OpenSpec changes, driven from his phone. This suite asks one
question of one phase of the work: **what does Nacre need so this phase feels better than doing it in the
Claude Code app or the Codex app?** It answers with ranked, evidenced findings, not a feature wishlist.

## Input

`/devux-review <phase> [focus]`. Phases (see `rubric.md` for what each covers):

- `ideation`: from a loose idea to a signed-off spec or bead (brainstorming, drafting, intake, approval).
- `confusion`: any moment Ryder or the agent doesn't know what's going on.
- `execution`: rounds, work cards, Needs you, landing.
- `review`: sign-off, human checks, evidence, archive.
- `return`: coming back after hours or days; "where was I?".

A free-text focus narrows it ("on the folded phone", "the intake sheet").

## Steps

1. **Ground.** Build a short brief (under 60 lines) for the reviewers. Base it on the accepted specs and active
   changes that govern the phase (`openspec/specs/*/spec.md`, `openspec/changes/*`), the app and nacred files
   that draw it, the chat briefing and prompts (`bridge/internal/chat/briefing.go`, `drafts.go`), and
   `bd memories`. List what exists today with file paths. Say what is out of scope, and what an earlier
   report in `docs/devux/` already found.
2. **Fan out.** Start one Agent for each lens in `lenses/`, all in one message so they run in parallel.
   Give each one the brief, its lens file, `rubric.md` and the phase. Run the `second-harness` lens with
   the `codex:codex-rescue` agent when Codex is available, and skip it otherwise. Reviewers read code and
   specs. They change nothing, answer no decision card and touch no device.
3. **Merge.** Dedupe across lenses: keep the best-evidenced finding and list which lenses raised it, since
   agreement between lenses is signal. Drop anything that:
   - already exists and works, unless it is undiscoverable (then reframe it as discoverability);
   - an active change or open bead already covers (cite it instead);
   - goes against Ryder's standing constraints in `rubric.md`.
4. **Verify.** For each finding still standing, check its claim against the code or spec yourself (one grep
   or read). A finding whose claim is wrong is dropped. A finding that holds only partly is reworded.
5. **Rank.** Use the rubric's score (user cost × frequency, divided by effort) and group the findings into
   Now, Next and Later. Name a "first slice" that fits one OpenSpec change.
6. **Report.** Write `docs/devux/<YYYY-MM-DD>-<phase>.md` following the report outline in `rubric.md`. Send
   it to Ryder with SendUserFile, and answer the question in chat in a short summary.
7. **Offer, don't file.** Findings are proposals. Don't create beads or OpenSpec changes until Ryder picks
   which ones. Then the first slice goes through `/opsx:propose` (or New effort), and the rest become beads
   related to the review's bead.

## Cost

Six reviewers read code once each. Keep each brief tight and ask for at most 12 findings per lens. Don't
rerun a lens on the same phase unless the code changed.
