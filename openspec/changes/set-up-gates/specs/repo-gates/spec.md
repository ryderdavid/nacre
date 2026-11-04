## ADDED Requirements

### Requirement: nacred drafts and saves nacre.yaml
nacred SHALL draft a `nacre.yaml` for a repository that has none with one read-only Claude call capped
at $1.50, and SHALL report why a draft can't be used as written. nacred SHALL save an approved file only
when it parses and no `nacre.yaml` exists, and SHALL commit that file alone on the checkout's branch.

#### Scenario: Draft
- **WHEN** the app asks for a draft for a repository without `nacre.yaml`
- **THEN** nacred returns the file and notes, with a problem when the file doesn't parse

#### Scenario: Save commits only the file
- **WHEN** Ryder saves an approved `nacre.yaml` while other edits are staged
- **THEN** the new commit holds only `nacre.yaml` and the other edits stay staged

#### Scenario: Never overwrite
- **WHEN** the repository already has a `nacre.yaml`
- **THEN** drafting and saving fail with a conflict
