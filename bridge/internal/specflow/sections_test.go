package specflow

import (
	"strings"
	"testing"
)

func TestTaskSectionsAcceptBeadReferenceTables(t *testing.T) {
	// A repository whose tasks.md is a Beads reference table instead of a
	// checklist (dta-sa4's draft): the table of existing beads generates
	// nothing; the table of new steps is the section of work.
	body := `## 1. Authoritative implementation plan

Repository instructions require Beads for task state. This artifact maps the change to durable issues; it intentionally contains no duplicate Markdown checklist. Query current state with ` + "`" + `bd show dta-7ou` + "`" + ` and ` + "`" + `bd ready` + "`" + `. Claim and close tasks in Beads. Do not run checklist-based OpenSpec apply automation against this reference table.

| Order | Beads issue | Deliverable |
| --- | --- | --- |
| 1 | ` + "`" + `dta-7ou.1` + "`" + ` | Prove Android wire compatibility with the real Mac app |
| 2 | ` + "`" + `dta-7ou.2` + "`" + ` | Inventory the complete iOS remote image interface |
| 3 | ` + "`" + `dta-7ou.3` + "`" + ` | Create native Android shell and mandatory Mac connection |
| 4 | ` + "`" + `dta-7ou.4` + "`" + ` | Implement authenticated encrypted app-private vault |
| 5 | ` + "`" + `dta-7ou.5` + "`" + ` | Implement remote generation lifecycle and private result save |
| 6 | ` + "`" + `dta-7ou.6` + "`" + ` | Build full applicable image control interface |
| 7 | ` + "`" + `dta-7ou.7` + "`" + ` | Verify release on Android against official Draw Things Mac app |
| 8 | ` + "`" + `dta-7ou.8` + "`" + ` | Mock up and implement the Material You interface |
| 9 | ` + "`" + `dta-7ou.9` + "`" + ` | Add adaptive foldable and tablet layouts |
| 10 | ` + "`" + `dta-7ou.10` + "`" + ` | Establish persistent release signing and verify the APK |
| 13 | ` + "`" + `dta-7ou.13` + "`" + ` | Compact image-centric workspace, overlay controls and carousel |
| 14 | ` + "`" + `dta-7ou.14` + "`" + ` | Mac-rendered streaming preview relay and temporary Android live card |
| 16 | ` + "`" + `dta-sa4` + "`" + ` | Zoomable live previews: front-card tap-to-expand, shared zoom state, hand-off to saved image |

Dependencies live in Beads. The wire proof and parity inventory can begin independently; the app acceptance issue depends on the completed editor path. Keep this OpenSpec change open until the implementation epic and all acceptance requirements are fulfilled.

Seed defaults, carousel elevation/motion, and progress spacing are tracked by
Beads ` + "`" + `dta-7ou.15` + "`" + `; validation evidence is in ` + "`" + `android/VERIFICATION.md` + "`" + `.

## 2. Zoomable live previews (` + "`" + `dta-sa4` + "`" + `) scope and gates

Work items to track under ` + "`" + `dta-sa4` + "`" + ` (as Beads notes or child issues, not Markdown checkboxes):

| Step | Deliverable |
| --- | --- |
| a | Extract shared zoom/pan (1x–6x, clamp ±size*(zoom-1)/2) with hoisted state from ` + "`" + `PrivateGalleryImage` + "`" + ` |
| b | Generalize ` + "`" + `FullScreenImage` + "`" + ` to vault-id or live-batch-index sources, keeping SecureOn and ` + "`" + `full-screen-image` + "`" + ` |
| c | ` + "`" + `BatchPreviewStack` + "`" + ` front-card expand callback (non-front tap still selects; waiting slot inert), wired in carousel live page and ` + "`" + `Preview()` + "`" + ` |
| d | Expose saved record id per batch index from the save path; viewer hand-off keeps zoom, else closes |
| e | Device tests and ` + "`" + `android/VERIFICATION.md` + "`" + ` evidence |

Gate commands (run from ` + "`" + `android/` + "`" + `):

- ` + "`" + `./gradlew :app:testDebugUnitTest` + "`" + `
- ` + "`" + `./gradlew :app:assembleDebug` + "`" + `
- ` + "`" + `./gradlew :app:connectedDebugAndroidTest -Pandroid.testInstrumentationRunnerArguments.class=com.ryderdavid.drawremote.ImageWorkspaceDeviceTest,com.ryderdavid.drawremote.BatchPreviewDeviceTest` + "`" + `

Manual phone verification of streamed zoom requires dta-zm4 (live previews visible again); record the result in ` + "`" + `android/VERIFICATION.md` + "`" + `.
`
	known := func(id string) bool { return strings.HasPrefix(id, "dta-7ou") || id == "dta-sa4" }
	out, sections, e := prepareSections(body, nil, nil, nil, known)
	if e != nil {
		t.Fatal(e)
	}
	if len(sections) != 1 || !strings.HasPrefix(sections[0].Title, "2. Zoomable live previews") || !strings.Contains(sections[0].Body, "BatchPreviewStack") {
		t.Fatalf("want only the new-work section, got %+v", sections)
	}
	if strings.Count(out, "nacre-section:") != 1 || !strings.Contains(out, "| 16 | `dta-sa4` |") {
		t.Fatalf("reference table must be kept unmarked:\n%s", out)
	}
	// Without a board that knows those beads, section 1 is a task table.
	if _, sections, e = prepareSections(body, nil, nil, nil, nil); e != nil || len(sections) != 2 {
		t.Fatalf("unknown references: %v %d", e, len(sections))
	}
	for _, bad := range []string{
		"## Only references\n| Order | Bead |\n| --- | --- |\n| 1 | `dta-7ou.1` |",
		"## Prose only\nNothing to do here.",
	} {
		if _, _, e := prepareSections(bad, nil, nil, nil, known); e == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	// Tracked work can't quietly turn into a reference table.
	old := []Section{{ID: "sfknown001", Title: "One", BeadID: "NAC-2", Body: "- [ ] one"}}
	ref := "<!-- nacre-section: sfknown001 -->\n## One\n| Bead |\n| --- |\n| `dta-7ou.1` |\n## Two\n- [ ] two"
	if _, _, e := prepareSections(ref, old, nil, nil, known); e == nil {
		t.Fatal("work section silently became a reference table")
	}
}
