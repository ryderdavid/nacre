# Finish to main

## Why
Setting a task in the app rarely ended in finished work. Implement started a plain run in the checkout:
nothing checked it, committed it, landed it or closed its bead, so beads sat in progress. Rounds did
all of that but hid behind "Work several beads until done" and waited at "ready to land". Epics and
their specification changes had no last step at all: nothing archived a change, merged an epic's work
or closed the epic, so finished epics stayed open on the board.

## What Changes
- Implement on a bead leads with Work until done: a one-bead round that lands when it passes.
- Every epic has a branch. Its beads' rounds land there, and finishing the epic merges the branch
  into the landing target with every gate and the build, pushes it and closes the epic.
- Finishing a specification change's epic archives the change in the same round.
- Needs you offers Finish for done epics and asks about beads stuck in progress.
- Landing stops at pushing; building and publishing releases belong to the repository's CI.

## Capabilities

### New Capabilities
- `epic-completion`: landing work where it belongs and finishing epics and their changes.

### Modified Capabilities

## Impact
nacred rounds, attention and specflow; the app's Implement sheet, epic detail, round cards and Needs
you; `docs/API.md` and the README.
