## MODIFIED Requirements

### Requirement: Ready to archive is visible
When every current section of the approved revision has landed evidence with matching provenance, no section bead is reopened or awaiting a human check, and no found bead under the epic is open, the app SHALL report the change as ready to archive and SHALL offer Finish, which archives the change and closes its epic through a finishing round. It SHALL NOT archive or close the epic without that action.

#### Scenario: All sections landed
- **WHEN** the last section's round lands
- **THEN** Needs you shows the change as ready to archive, with Finish: archive and merge and Open change

#### Scenario: A section was reopened
- **WHEN** a landed section bead is reopened
- **THEN** the change is no longer reported as ready to archive

#### Scenario: Other specifications moved on
- **WHEN** another change is added or archived after approval, so the approval is stale, but this change's own files are still exactly the approved ones
- **THEN** the change is still reported as ready to archive, saying what changed elsewhere
