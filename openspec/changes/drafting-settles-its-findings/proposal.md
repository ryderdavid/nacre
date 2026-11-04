## Why
After its passes, drafting stopped on leftover medium findings, and the guide wrote the fixes into the
owner's feedback box as "Your decisions" and offered "Revise · up to $5" on a draft he had never seen.
The author can make those calls itself; the owner should only be asked what only he can decide, and
buttons shouldn't read like purchases.

## What Changes
- The reviewer marks each finding as one the author can fix or one that needs the owner, with choices.
- The loop keeps drafting while findings the author can fix remain (up to five passes), and stops early
  only for owner decisions. Low findings ride along but don't cause a pass.
- The guide never writes feedback in the owner's name: an owner decision is offered as choices, and
  leftovers the author can fix become "Finish the draft" with nothing prefilled.
- No dollar amounts on buttons or action copy. The action limit is a runaway guard of $30, well above
  ordinary drafting.

## Capabilities
### New Capabilities
None.
### Modified Capabilities
- `spec-guided-work`: the author/reviewer loop settles its own findings and asks the owner only real decisions.

## Impact
`bridge/internal/specflow` (types.go, author.go, guide.go, tests); the app's change screen, guide,
policy, intake and assistant copy.
