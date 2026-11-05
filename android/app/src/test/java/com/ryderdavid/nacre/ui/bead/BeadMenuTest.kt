package com.ryderdavid.nacre.ui.bead

import com.ryderdavid.nacre.data.model.ChangeEnvelope
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.SpecChange
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/** Develop a proposal lives in the bead's overflow menu, for unlinked beads only. */
class BeadMenuTest {
    private val unlinked = Issue(id = "NAC-1")
    private val available = ChangeEnvelope(available = true)

    @Test fun theOverflowMenuOffersDevelopAProposalOnAnUnlinkedBead() {
        val develop = offersDevelop(unlinked, onBoardChange = false, envelope = available)
        assertTrue(develop)
        val menu = beadMenu(unlinked, develop, found = false)
        assertTrue(BeadMenuItem.Develop in menu)
        assertEquals("Develop a proposal", BeadMenuItem.Develop.label)
        assertEquals(
            listOf(BeadMenuItem.CopyId, BeadMenuItem.Develop, BeadMenuItem.RunManually, BeadMenuItem.Close),
            menu,
        )
    }

    @Test fun aLinkedBeadOffersNoDevelop() {
        // Its own spec workflow, a change on the board, or one nacred reports: nothing to develop.
        assertFalse(offersDevelop(Issue(id = "NAC-2", labels = listOf("specflow:x")), false, available))
        assertFalse(offersDevelop(unlinked, onBoardChange = true, envelope = available))
        assertFalse(offersDevelop(unlinked, false, ChangeEnvelope(available = true, change = SpecChange(id = "c", seedId = "NAC-1", phase = "drafting"))))
        assertFalse(BeadMenuItem.Develop in beadMenu(unlinked, develop = false, found = true))
    }

    @Test fun noDevelopWhereSpecWorkIsntAvailable() {
        assertFalse(offersDevelop(unlinked, false, ChangeEnvelope(available = false)))
        assertFalse(offersDevelop(unlinked, false, null))
        assertFalse(offersDevelop(null, false, available))
    }

    @Test fun aClosedBeadReopensAndAMissingOneOnlyCopies() {
        assertEquals(BeadMenuItem.Reopen, beadMenu(Issue(id = "NAC-3", status = "closed"), false, false).last())
        assertEquals(listOf(BeadMenuItem.CopyId), beadMenu(null, develop = true, found = true))
    }
}
