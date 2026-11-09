# Lens: visual thinking

You review how Nacre lets Ryder and the agent think in pictures during the phase: mockups, diagrams,
canvases, prototypes and marked-up screenshots. Read the mockup path (`bridge/internal/chat/briefing.go`
"Showing mockups", `A/data/chat/Mockups.kt`, `A/ui/common/MockupCard.kt`) and how chat replies are rendered
(`A/ui/chat/Conversation.kt`, `MarkdownText`).

Questions:

- **Canvas or snapshots?** Can one design surface stay open and update in place as the conversation goes,
  with versions he can step through and compare, or does each revision become a new card further down the
  chat?
- **Diagrams.** Can the agent show an architecture, flow, sequence or state diagram natively (for example a
  Mermaid fence), or only by hand-writing HTML or SVG? Are diagrams legible on the cover screen, and can he
  zoom in?
- **Views from the project's own data.** Does the agent have standard views built from data only Nacre
  holds? Examples:
  - the spec, requirement, change, bead and round map;
  - a round's dependency graph;
  - "how does this work" across nacred and the app.
- **Ryder's own input.** Can he sketch, draw over a screenshot, or pin a comment on part of a mockup and send
  it back? Can he tap a choice inside a visual?
- **Prototypes.** Can a mockup be a tap-through prototype that keeps its state? Does it survive when the
  card scrolls away?
- **Carry-through.** Does an agreed visual reach the draft, the bead, the author and the change's design.md?
  Can the approve card show it?
- **Posture.** What does each of these look like folded at about 400 dp versus unfolded at about 900 dp or
  in tabletop?

Benchmark against what you know of:

- Claude apps: Artifacts with versions, native Mermaid, inline visual widgets that answer back
  (sendPrompt), and design or canvas surfaces.
- The Codex app: image input, and any visual planning surfaces it has.

Say where Nacre could lead, for example views built from beads and OpenSpec that no general harness can
draw.

Return at most 10 findings, using the schema in `rubric.md`.
