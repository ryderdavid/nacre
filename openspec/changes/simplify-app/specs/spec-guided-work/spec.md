## ADDED Requirements

### Requirement: Approve and run
A change awaiting approval SHALL offer Approve and run as its primary action, with Revise beside it.
Approve and run SHALL approve the reviewed revision and, once its epic and section beads are prepared,
start Run epic for it without another action. If the run cannot start, the change SHALL show why and
offer Run epic again.

#### Scenario: One approval to finished work
- **WHEN** Ryder taps Approve and run on a reviewed change with three task sections
- **THEN** the epic and its three section beads are prepared and one round starts working all three

### Requirement: Plain-language change phases
The app SHALL describe a change in four phases: Planning while it is drafted, reviewed or prepared,
Ready for your approval, Working once approved, and Done once archived. A change that needs attention
(interrupted, blocked by review, or stale) SHALL show one Needs attention banner naming the action that
fixes it.

#### Scenario: Working
- **WHEN** an approved change's epic is running
- **THEN** the change reads Working with its sections' progress

#### Scenario: Interrupted
- **WHEN** a change's proposal work was interrupted
- **THEN** it shows Needs attention with Resume
