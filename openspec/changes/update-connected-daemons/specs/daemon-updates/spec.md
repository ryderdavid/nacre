## ADDED Requirements

### Requirement: Coordinated Settings action
Settings SHALL offer one action for updating connected computers in the existing computer section and SHALL display a separate result for every paired host.

#### Scenario: Mixed availability
- **WHEN** a user requests updates with reachable, offline and older computers paired
- **THEN** reachable supported hosts receive independent requests and the others show actionable offline or manual-upgrade messages without blocking the batch

#### Scenario: Navigation and connection loss
- **WHEN** a user leaves Settings or the daemon restarts during updating
- **THEN** status is reconciled on return and disconnection is never shown as successful completion

### Requirement: Trusted stable release
The daemon MUST accept only authenticated update requests for the fixed Nacre repository's latest published stable release, resolve the exact commit, and build bounded isolated source with explicit version and commit metadata.

#### Scenario: No release or invalid content
- **WHEN** no stable release exists, the tag is invalid, or source extraction/build fails
- **THEN** the installed executable and service remain unchanged and a useful failure is returned

#### Scenario: Already current or newer
- **WHEN** the resolved release matches the running provenance or is older than the running version
- **THEN** the service is not restarted and the result explains its current status

### Requirement: Work preservation
The daemon MUST atomically reserve maintenance against new mutation requests and MUST refuse updates while owned runs, nonterminal rounds or their unfinished background cleanup, assistant calls or specification jobs are active.

#### Scenario: Concurrent start
- **WHEN** implementation and update requests arrive concurrently
- **THEN** only the admitted operation starts and the other receives a retryable busy result

#### Scenario: Duplicate update
- **WHEN** an update request is repeated while an update is active
- **THEN** the existing operation is returned without launching another helper

#### Scenario: Historical result after lost response
- **WHEN** an update POST response is lost and status still identifies the operation observed before the request
- **THEN** the app reports the new request as unconfirmed and SHALL NOT present the historical result as its success

#### Scenario: Terminal cleanup remains active
- **WHEN** a landed or cancelled round still has background work running
- **THEN** updating is refused until that work finishes

### Requirement: Verified replacement and rollback
A separate helper SHALL keep a rollback executable, replace atomically, restart the correct installed LaunchAgent and verify authenticated health for exact version, commit and host identity before reporting updated.

#### Scenario: Replacement fails health
- **WHEN** the new daemon does not pass health verification
- **THEN** the helper restores the prior binary, restarts and verifies it, and distinguishes restored service from failed recovery

#### Scenario: Helper interruption
- **WHEN** the helper cannot finish within its bounded operation window
- **THEN** persisted status reports interruption and recovery guidance without claiming success
