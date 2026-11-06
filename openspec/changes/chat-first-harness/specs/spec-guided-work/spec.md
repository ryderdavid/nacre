# Spec Delta

## MODIFIED Requirements

### Requirement: Approve and run
A change awaiting approval SHALL raise a reviewed-action decision, offering:
- **Approve, run, I'll land** as the recommended option: it approves, runs, and stops before landing
- **Approve, run and land to main**: it approves, runs, and lands and pushes when the gates pass
- **Approve only**: it approves without starting work
- **Reject** with a note, which sends the change back to its author

The same options and recommendation SHALL appear wherever the decision is shown.

Each option SHALL state its consequences. Approve, run, I'll land SHALL approve the reviewed revision and, once its
epic and section beads are prepared, start Run epic for it without another action. If the run cannot
start, the card SHALL show why and offer Run epic again.

#### Scenario: One approval to finished work
- **WHEN** Ryder picks Approve, run, I'll land on a reviewed change with three task sections
- **THEN** the epic and its three section beads are prepared, one round starts working all three, and it stops before main

#### Scenario: Landing chosen up front
- **WHEN** Ryder picks Approve, run and land to main
- **THEN** the round lands to main when its gates pass, as the card said it would
