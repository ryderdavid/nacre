package com.ryderdavid.nacre.ui.board

import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.ChangesList
import com.ryderdavid.nacre.data.model.IntakeVerdict
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.SectionSummary
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class SpecSurfacesTest {
    private val changes = ChangesList(
        openspec = true,
        changes = listOf(
            ChangeSummary(
                "c1", "retain-drafts", phase = "ready", revision = "r", approvedRevision = "r", epicId = "E", seedIds = listOf("E"),
                sections = listOf(SectionSummary("s1", "Save", "S1", "landed"), SectionSummary("s2", "Restore", "S2", "none")),
            ),
            ChangeSummary("c2", "chat", phase = "awaiting_approval", seedIds = listOf("N")),
        ),
    )
    private fun issue(id: String, status: String = "open", labels: List<String> = emptyList()) = Issue(id = id, status = status, labels = labels)

    @Test fun anEpicWhoseSectionsWaitOnSignOffSaysHowManyToCheck() {
        val chip = specChip(issue("E"), changes, toCheck = setOf("S2"))!!
        assertEquals("1 to check", chip.text)
        assertEquals(ChipTone.CHECK, chip.tone)
        assertEquals("Ready to implement", specChip(issue("E"), changes, toCheck = setOf("X"))!!.text, "only its own sections count")
    }

    @Test fun oneSpecChipPerCard() {
        assertEquals(SpecChip("Ready to implement", ChipTone.SPEC, changes.changes[0].sections), specChip(issue("E"), changes))
        assertEquals(ChipTone.NEEDS_YOU, specChip(issue("N"), changes)!!.tone)
        assertEquals("§ 1 of retain-drafts · merged", specChip(issue("S1"), changes)!!.text)
        assertEquals("Closed, not verified", specChip(issue("S2", status = "closed"), changes)!!.text)
        assertEquals("Changes retain-drafts", specChip(issue("F", labels = listOf("found:c1", "found-amend")), changes)!!.text)
        assertEquals("Added to retain-drafts", specChip(issue("G", labels = listOf("found:c1")), changes)!!.text)
        assertEquals("Touches a spec", specChip(issue("T", labels = listOf("spec-touch:boards")), changes)!!.text)
        assertNull(specChip(issue("plain", labels = listOf("ux")), changes))
        assertNull(specChip(issue("E"), null), "older bridges show cards as before")
        assertTrue(isSpecLabel("specflow:c1") && isSpecLabel("found-amend") && !isSpecLabel("ux"))
    }

    @Test fun intakeRouting() {
        val amend = IntakeVerdict("amend", changeName = "rounds", rationale = "Changes retry.", activeChanges = listOf("rounds", "chat"))
        assertEquals(EffortRoute.Amend("rounds"), EffortRoute.of(amend))
        assertEquals("Create and develop · up to \$5", EffortRoute.of(amend).confirmLabel())
        assertEquals("Intake: amend rounds, because Changes retry.", EffortRoute.of(amend).feedback(amend))
        assertEquals("Intake: a new change, chosen by Ryder over intake's verdict", EffortRoute.NewChange.feedback(amend))
        assertEquals("amend", EffortRoute.Amend("rounds").hint()!!.verdict)
        assertNull(EffortRoute.Bead.hint())
        assertEquals("Create bead", EffortRoute.Bead.confirmLabel())
        assertEquals(EffortRoute.NewChange, EffortRoute.of(IntakeVerdict("amend")), "an amend without a change becomes new")
        assertEquals(EffortRoute.Covered, EffortRoute.of(IntakeVerdict("covered")))
        assertNull(EffortRoute.Amend("rounds").blockedBy(amend))
        val busy = amend.copy(targetActive = "Round R1 is still active.", targetTracked = true)
        assertTrue(EffortRoute.Amend("rounds").blockedBy(busy)!!.contains("Round R1 is still active"))
        assertNull(EffortRoute.Amend("chat").blockedBy(busy), "only the busy change waits")
    }
}
