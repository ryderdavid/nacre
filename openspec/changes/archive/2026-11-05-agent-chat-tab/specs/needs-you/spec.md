## MODIFIED Requirements

### Requirement: The assistant starts from Needs you
Chat SHALL see the same Needs you list through nacred's API or the `nacred` command, which its briefing names. The briefing SHALL tell it to read that list first when Ryder asks what to work on or what needs him.

#### Scenario: What should I work on next
- **WHEN** Ryder asks Chat what to work on next while a proposal awaits approval
- **THEN** the answer mentions the waiting proposal before suggesting new work

#### Scenario: Mac assistant
- **WHEN** Chat runs on the Mac and answers what needs Ryder
- **THEN** it reads the attention list from nacred on that computer before answering
