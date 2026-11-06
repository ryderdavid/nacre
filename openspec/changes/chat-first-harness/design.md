# Design

## Context

See proposal.md for why. What shapes the approach:

- **Chat can already do the work.** It runs the full Claude Code or Codex harness on the Mac through
  nacred, with host permission prompts answered from the phone. The agent reaches nacred's API with
  ordinary credentials, so today it can approve, land, finish and raise limits without Ryder.
- **Attention items already carry their options.** `GET /v1/attention` returns items whose options are
  server-defined (method, path and body), and the Inbox performs them generically.
- **The board already has a good compact mode.** It shows one lane at a time, with lane tabs and
  counts, a drop dock, and Undo. Dragging starts on long-press. Dropping on Done closes the bead.
- **Repositories are grouped across computers by git remote**, while ids stay scoped to their host.
- **Hinge handling already exists**: the app's PaneLayout helper.
- **The target device is a Galaxy Z Fold.** The cover screen is compact width. The inner screen is about
  840dp wide, which leaves about 728dp after the 96dp rail and the divider.

The plan was reviewed in three rounds by an independent Codex design review, using a design-critique
method and an accessibility pass. Its findings shaped decisions 1, 2, 4, 5, 6 and 8 below.

## Goals / Non-Goals

**Goals:**
- One place to ask for work (Chat) and two places to see the truth (Spec and Beads).
- Owner gates that are explicit, specific to one action, and safe against agent mistakes.
- Deterministic controls that keep working when the agent can't: stop, cancel, retry, recovery and Settings.
- A first-party Material 3 Expressive look on both of the Fold's screens.

**Non-Goals:**
- Sandboxing the agent. Protection covers nacred operations only.
- Changing how rounds, Beads or OpenSpec behave.
- Removing API routes.

## Decisions

1. **Protected actions and a chat-scoped token.**
   - nacred mints a token per chat turn and puts it in the turn's environment.
   - Protected routes reject that token with 403 `needs_decision`.
   - Device tokens keep working everywhere, so older apps are unaffected.
   - *Alternative:* trust the agent's restraint through its briefing. Rejected: a briefing guards
     nothing against mistakes or prompt injection.
   - *Alternative:* a separate owner credential that the phone would need for everything. Rejected:
     it breaks older apps, and it is broader than needed.
2. **Decisions are built and executed by nacred.**
   - The agent asks for a decision by kind and target. nacred computes the exact action, the revision,
     the consequences and the expiry.
   - On approval from a paired device, nacred revalidates and executes, idempotent by decision id.
   - Attention items become decisions by mapping each item's server-defined options. The decision
     store is generalized from the permission-request store, which already has paired-device answers,
     timeouts and notifications.
   - *Alternative:* the app calls the protected route itself after approval. Rejected: it spreads the
     binding, revalidation and idempotency across clients.
3. **Two levels, three destinations.**
   - Projects is the home. The workspace has Chat, Spec and Beads.
   - On wide layouts, the rail chooses the layout (Chat alone, or Chat beside Spec or Beads) instead
     of a separate Spec | Beads toggle in the pane.
   - *Alternative:* keep four tabs and add panels. Rejected: it keeps the button-heavy model the change exists to remove.
4. **Geometry by usable width.**
   - The divider presets are computed from usable width and the pane minimums (Chat 360dp, pane
     320dp), not from fractions. At 728dp, a one-third preset would break both minimums.
   - Below 680dp of usable width, one pane shows at a time.
   - Focus is the way to see a full kanban.
5. **Keep the compact board.**
   - Long-press stays dedicated to dragging. Ask about this moves to the peek and the card's menu.
   - Done means closed, never signed off.
6. **Execution-tied agent states.**
   - The app shows Working while the process runs, and states silence as an observation.
   - "May be stalled" comes only from nacred's stall signal, which uses its configurable threshold
     (15 minutes by default).
7. **Material 3 Expressive, with jobs.**
   - Components are matched to jobs:
     - navigation: the short navigation bar or the rail
     - selection: connected button groups, for the Projects filter and the document tabs
     - creation: the "New Claude chat" split button
     - reading: the floating toolbar
     - live work: the expressive loading indicator
   - Expressive shapes appear on identity (project and thread avatars), never on state.
   - Cards you act on keep a stable geometry.
   - Dynamic color, with the existing mother-of-pearl teal palette as the fallback.
8. **Palettes are generated, not hand-picked.**
   - Each fixed palette is a hue and chroma per tonal palette (primary, secondary, tertiary, neutral,
     neutral variant), turned into roles with Material's tone mapping. The light and dark versions come
     from the same palette.
   - Ocean keeps primary, secondary and tertiary all in blue hues (royal, steel and sky), so the app reads
     as blue throughout.
   - A unit test keeps every palette inside the contrast contract.
   - *Alternative:* hand-tuned hex sets. Rejected: they drift between roles and between light and dark.
9. **Accessibility is part of the contract** (the app-accessibility spec). It is validated on the Fold
   with TalkBack, Switch Access and a keyboard before sign-off.

## Risks / Trade-offs

- **Actions are harder to discover without buttons.**
  → Suggestion chips appear on an empty composer, decision cards bring owner actions to Ryder, and Ask
  about this sits on every object.
- **Typing on the cover screen is slow.**
  → Voice input, reference chips, and suggestions that fill the composer.
- **The agent could misreport what it did.**
  → Panels update live from nacred, and decision cards show nacred's own result, not the agent's.
- **A chat-scoped token doesn't stop an agent reading the device token from disk.**
  → This is stated plainly in the spec. Claude Code's classifier and host prompts still apply. A
  follow-up could move the device token out of the agent's reach.
- **The change removes a lot of UI at once, and regressions could hide in that removal.**
  → The removal comes last, after the new workspace and cards are in and tested. Every removed flow is
  checked against the inventory and must be covered by chat or a card.

## Migration Plan

1. nacred ships first, keeping every existing route: the token scopes, decisions and summary endpoints.
   The current app keeps working against it.
2. The app ships the new workspace behind a setting ("Chat-first layout"), defaulting on for the
   developer build, so the old shell can be compared for one release.
3. Once the device checks pass, the old shell, tabs, screens and sheets are removed, and the setting
   goes with them.
4. Rolling back means turning the setting off (step 2), or installing the previous APK. nacred stays
   compatible with both.
