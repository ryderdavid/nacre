package com.ryderdavid.nacre.data.board

import com.ryderdavid.nacre.data.bridge.BridgeException
import com.ryderdavid.nacre.data.model.ChangeDraft
import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.ChangesList
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.ui.board.ChipTone
import com.ryderdavid.nacre.ui.board.specChip
import kotlinx.coroutines.runBlocking
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** pending-proposal-placeholders §4: the failed draft's error state, Resume and Dismiss. */
class DraftErrorStateTest {
    private fun issue(id: String) = Issue(id = id, title = "t $id", status = "open", type = "feature")

    private val board = BoardIndex(listOf(issue("P1"), issue("P2"), issue("D1"), issue("W")))

    /** Two non-waiting proposals started at 03:00 (P1) and 01:00 (P2), one waiting started the day before (W), and the draft started at 02:00. */
    private fun lane(draft: ChangeSummary) = ChangesList(
        openspec = true, total = 4,
        changes = listOf(
            ChangeSummary("c-new", "newer", phase = "reviewing", seedIds = listOf("P1"), createdAt = "2026-11-01T03:05:00Z", startedAt = "2026-11-01T03:00:00Z", updatedAt = "2026-11-01T03:10:00Z"),
            draft,
            ChangeSummary("c-old", "older", phase = "reviewing", seedIds = listOf("P2"), createdAt = "2026-11-01T01:05:00Z", startedAt = "2026-11-01T01:00:00Z", updatedAt = "2026-11-01T04:00:00Z"),
            ChangeSummary("c-wait", "waiting", phase = "awaiting_approval", seedIds = listOf("W"), createdAt = "2026-10-31T00:05:00Z", startedAt = "2026-10-31T00:00:00Z", updatedAt = "2026-10-31T00:10:00Z"),
        ),
    )

    private val pending = ChangeSummary(
        "c-draft", "", phase = "drafting", seedIds = listOf("D1"),
        startedAt = "2026-11-01T02:00:00Z", updatedAt = "2026-11-01T02:00:00Z", draft = ChangeDraft(ChangeDraft.PENDING),
    )
    private val failed = pending.copy(
        phase = "needs_attention", error = "The author's output failed strict validation.", updatedAt = "2026-11-01T05:00:00Z",
        draft = ChangeDraft(ChangeDraft.FAILED, "The author's output failed strict validation."),
    )
    /** After Dismiss: nacred stops reporting the draft; the change keeps its phase and error. */
    private val dismissed = failed.copy(draft = null)

    private fun proposals(changes: ChangesList) =
        board.columns(GroupBy.WORKFLOW, BoardFilter(), changes = changes).first { it.key == Lane.PROPOSED.key }

    /** A stand-in for the computer: what the changes summary reports, and how it answers Dismiss and Resume. */
    private class Computer(var summary: ChangesList) {
        val sent = mutableListOf<String>()
        var dismissWith: (() -> ChangesList)? = null
        var resumeWith: (() -> ChangesList)? = null
        val notices = mutableListOf<String>()
        val actions = DraftActions(
            dismiss = { id -> sent += "dismiss $id"; summary = dismissWith!!() },
            resume = { bead -> sent += "resume $bead"; summary = resumeWith!!() },
            notify = { notices += it },
        )
    }

    @Test fun failedDraftShowsTheErrorStateInTheWaitingGroup() {
        val pendingKeys = proposals(lane(pending)).items.map { it.key }
        assertEquals(listOf("W", "P1", "draft:c-draft", "P2"), pendingKeys)

        for (phase in listOf("needs_attention", "interrupted")) {
            val col = proposals(lane(failed.copy(phase = phase)))
            // Not gone: still exactly one item for the change, and it moved to the top (waiting) group,
            // ordered there by started_at (02:00 on Nov 1 is newer than W's Oct 31).
            assertEquals(listOf("draft:c-draft", "W", "P1", "P2"), col.items.map { it.key }, phase)
            val placeholder = assertIs<LaneItem.DraftPlaceholder>(col.items.first())
            assertTrue(placeholder.failed)
            assertEquals("The author's output failed strict validation.", placeholder.draft.message)
            assertTrue(placeholder.resumable, "the change's existing Resume is offered in $phase")
            assertTrue(placeholder.dismissable)
            assertEquals("D1", placeholder.resumeBead)
            assertNull(placeholder.title)
            assertFalse(placeholder.clickable || placeholder.draggable)
            assertFalse(col.issues.any { it.id == "D1" }, "the seed's card stays hidden")
        }
    }

    @Test fun pendingDraftOffersNeitherResumeNorDismiss() = runBlocking<Unit> {
        val item = proposals(lane(pending)).items.filterIsInstance<LaneItem.DraftPlaceholder>().single()
        assertFalse(item.resumable)
        assertFalse(item.dismissable)
        val computer = Computer(lane(pending))
        assertFalse(computer.actions.resume(item))
        assertFalse(computer.actions.dismiss(item))
        assertTrue(computer.sent.isEmpty(), "nothing is sent for a pending draft")
    }

    @Test fun resumeReturnsThePendingPlaceholderToItsStartedAtPosition() = runBlocking<Unit> {
        val computer = Computer(lane(failed))
        // nacred accepts: the change is drafting again with created_at still empty, and a later update time.
        computer.resumeWith = { lane(pending.copy(updatedAt = "2026-11-01T06:00:00Z")) }
        val item = proposals(computer.summary).items.filterIsInstance<LaneItem.DraftPlaceholder>().single()

        assertTrue(computer.actions.resume(item))
        assertEquals(listOf("resume D1"), computer.sent, "the existing resume action, sent through the seed")
        val col = proposals(computer.summary)
        assertEquals(listOf("W", "P1", "draft:c-draft", "P2"), col.items.map { it.key }, "back between the proposals started before and after it")
        assertTrue(assertIs<LaneItem.DraftPlaceholder>(col.items[2]).draft.pending)
        assertTrue(computer.notices.isEmpty())
    }

    @Test fun refusedResumeKeepsThePlaceholderFailed() = runBlocking<Unit> {
        val computer = Computer(lane(failed))
        computer.resumeWith = { throw BridgeException("this change has no interrupted operation to resume", code = 409) }
        val before = proposals(computer.summary).items
        val item = before.filterIsInstance<LaneItem.DraftPlaceholder>().single()

        assertFalse(computer.actions.resume(item))
        assertEquals(before, proposals(computer.summary).items, "still failed, in the same place")
        assertEquals(listOf("Couldn't resume drafting: this change has no interrupted operation to resume"), computer.notices)
    }

    @Test fun dismissingShowsTheSeedsOrdinaryCardWithNeedsAttention() = runBlocking<Unit> {
        val computer = Computer(lane(failed))
        computer.dismissWith = { lane(dismissed) }
        val item = proposals(computer.summary).items.filterIsInstance<LaneItem.DraftPlaceholder>().single()

        assertTrue(computer.actions.dismiss(item))
        assertEquals(listOf("dismiss c-draft"), computer.sent)
        val col = proposals(computer.summary)
        assertTrue(col.items.none { it is LaneItem.DraftPlaceholder }, "the placeholder is gone")
        assertEquals(listOf("D1", "W", "P1", "P2"), col.items.map { it.key })
        val card = assertIs<LaneItem.Card>(col.items.first())
        assertEquals("t D1", card.title)
        assertTrue(card.clickable, "the ordinary card opens the bead, where Resume still is")
        val chip = specChip(card.issue, computer.summary)!!
        assertEquals("Needs attention", chip.text)
        assertEquals(ChipTone.NEEDS_YOU, chip.tone)
        assertTrue(computer.summary.changes.single { it.id == "c-draft" }.offersResume, "the change keeps its Resume")
        assertTrue(computer.notices.isEmpty())
    }

    @Test fun failedDismissLeavesTheErrorPlaceholderInPlace() = runBlocking<Unit> {
        val computer = Computer(lane(failed))
        computer.dismissWith = { throw java.net.ConnectException("refused") }
        val before = proposals(computer.summary).items
        val item = before.filterIsInstance<LaneItem.DraftPlaceholder>().single()

        assertFalse(computer.actions.dismiss(item))
        val after = proposals(computer.summary).items
        assertEquals(before, after)
        assertTrue(assertIs<LaneItem.DraftPlaceholder>(after.first()).failed, "still the error placeholder")
        assertEquals(1, computer.notices.size)
        assertTrue(computer.notices.single().startsWith("Couldn't dismiss the failed draft: Can't reach that computer"), computer.notices.single())

        // Dismissing again is safe: the retry goes through and the ordinary card appears.
        computer.dismissWith = { lane(dismissed) }
        assertTrue(computer.actions.dismiss(item))
        assertEquals(listOf("dismiss c-draft", "dismiss c-draft"), computer.sent)
        assertIs<LaneItem.Card>(proposals(computer.summary).items.first())
    }
}
