package com.ryderdavid.nacre.data.board

import com.ryderdavid.nacre.data.model.ChangeDraft
import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.ChangesList
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.ui.rounds.roundCandidates
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ProposalsLaneTest {
    private fun issue(id: String, status: String = "open", type: String = "feature", labels: List<String> = emptyList()) =
        Issue(id = id, title = "t $id", status = status, type = type, labels = labels)

    private val board = BoardIndex(listOf(issue("S1"), issue("S2"), issue("S3"), issue("E", type = "epic"), issue("A"), issue("F", labels = listOf("found:c4", "found-amend")), issue("X")))
    private val changes = ChangesList(
        openspec = true, total = 7,
        changes = listOf(
            ChangeSummary("c1", "drafts", phase = "reviewing", seedIds = listOf("S1"), updatedAt = "2026-11-01T02:00:00Z"),
            ChangeSummary("c2", "chat", phase = "awaiting_approval", seedIds = listOf("S2"), updatedAt = "2026-11-01T01:00:00Z"),
            ChangeSummary("c3", "export", phase = "covered", seedIds = listOf("S3"), updatedAt = "2026-11-01T03:00:00Z"),
            // A promoted change being amended by a new seed: the seed plans, the epic stays put.
            ChangeSummary("c4", "rounds", phase = "drafting", epicId = "E", seedIds = listOf("E", "A")),
        ),
    )

    @Test fun proposalsLaneHoldsSeedsBeingPlannedWaitingOnRyderFirst() {
        val cols = board.columns(GroupBy.WORKFLOW, BoardFilter(), changes = changes)
        assertEquals(Lane.PROPOSED.key, cols.first().key)
        assertEquals(listOf("S2", "S1", "A", "S3"), cols.first().issues.map { it.id }, "awaiting approval first, then being worked newest first, then covered")
        assertEquals("Showing 4 of 7 changes", cols.first().note)
        assertTrue(cols.first { it.key == "ready" }.issues.any { it.id == "E" }, "the epic stays in its ordinary lane")
        assertEquals(4, board.stats(BoardFilter(), changes)[Lane.PROPOSED])
    }

    @Test fun noLaneWithoutOpenSpecOrChangeRecords() {
        assertFalse(board.columns(GroupBy.WORKFLOW, BoardFilter()).any { it.key == Lane.PROPOSED.key })
        assertFalse(board.columns(GroupBy.WORKFLOW, BoardFilter(), changes = changes.copy(openspec = false)).any { it.key == Lane.PROPOSED.key })
        assertEquals(Lane.READY, board.workflowLane(board.byId["S1"]!!, null))
    }

    @Test fun promotedAndClosedSeedsLeaveTheLane() {
        val promoted = changes.copy(changes = listOf(ChangeSummary("c1", "drafts", phase = "ready", epicId = "S1", seedIds = listOf("S1"))))
        assertEquals(Lane.READY, board.workflowLane(board.byId["S1"]!!, promoted))
        val closed = BoardIndex(listOf(issue("S1", status = "closed")))
        assertEquals(Lane.DONE, closed.workflowLane(closed.byId["S1"]!!, changes))
        val staleApproved = changes.copy(changes = listOf(ChangeSummary("c1", "drafts", phase = "stale", epicId = "E1", seedIds = listOf("S1"))))
        assertEquals(Lane.READY, board.workflowLane(board.byId["S1"]!!, staleApproved), "a stale approved change is implementation, not planning")
    }

    @Test fun proposalsAreNotADropTargetAndOtherCallersKeepLaneOf() {
        assertNull(moveActionFor(Lane.PROPOSED.key))
        assertEquals(Lane.READY, board.laneOf(board.byId["S1"]!!))
    }

    @Test fun roundsLeaveOutProposalsAndPendingAmendments() {
        val ids = roundCandidates(board, changes).map { it.id }
        assertFalse("S1" in ids || "A" in ids, "beads being planned aren't work yet")
        assertFalse("F" in ids, "a pending amendment is not runnable")
        assertTrue("X" in ids)
    }

    // ---- Draft placeholders (pending-proposal-placeholders §3) ----

    private val pending = ChangeDraft(ChangeDraft.PENDING)
    private val draftBoard = BoardIndex(listOf(issue("P1"), issue("P2"), issue("D1"), issue("D2"), issue("W"), issue("R")))

    /** Three non-waiting changes started at 03:00 (c-new), 02:00 (c-draft, drafting) and 01:00 (c-old), and one waiting. */
    private fun draftChanges(draft: ChangeSummary) = ChangesList(
        openspec = true, total = 4,
        changes = listOf(
            ChangeSummary("c-new", "newer", phase = "reviewing", seedIds = listOf("P1"), createdAt = "2026-11-01T03:05:00Z", startedAt = "2026-11-01T03:00:00Z", updatedAt = "2026-11-01T03:10:00Z"),
            draft,
            ChangeSummary("c-old", "older", phase = "reviewing", seedIds = listOf("P2"), createdAt = "2026-11-01T01:05:00Z", startedAt = "2026-11-01T01:00:00Z", updatedAt = "2026-11-01T04:00:00Z"),
            ChangeSummary("c-wait", "waiting", phase = "awaiting_approval", seedIds = listOf("W"), createdAt = "2026-10-31T00:05:00Z", startedAt = "2026-10-31T00:00:00Z", updatedAt = "2026-10-31T00:10:00Z"),
        ),
    )
    private val drafting = ChangeSummary("c-draft", "", phase = "drafting", seedIds = listOf("D1"), startedAt = "2026-11-01T02:00:00Z", updatedAt = "2026-11-01T02:00:00Z", draft = pending)

    private fun proposals(changes: ChangesList, board: BoardIndex = draftBoard) =
        board.columns(GroupBy.WORKFLOW, BoardFilter(), changes = changes).first { it.key == Lane.PROPOSED.key }

    @Test fun placeholderReplacesTheSeedCard() {
        val col = proposals(draftChanges(drafting))
        assertEquals(listOf("W", "P1", "draft:c-draft", "P2"), col.items.map { it.key })
        assertFalse(col.issues.any { it.id == "D1" }, "the seed's card is hidden")
        assertEquals(4, col.total)
        val placeholder = assertIs<LaneItem.DraftPlaceholder>(col.items[2])
        assertEquals("c-draft", placeholder.changeId)
        assertTrue(placeholder.draft.pending)
    }

    @Test fun aChangeWithTwoSeedsIsOnePlaceholderAndNeitherCard() {
        val col = proposals(draftChanges(drafting.copy(seedIds = listOf("D1", "D2"))))
        assertEquals(1, col.items.count { it is LaneItem.DraftPlaceholder })
        assertEquals(listOf("D1", "D2"), col.items.filterIsInstance<LaneItem.DraftPlaceholder>().single().seedIds)
        val cards = col.items.filterIsInstance<LaneItem.Card>().map { it.issue.id }
        assertFalse("D1" in cards || "D2" in cards, "no seed appears twice: $cards")
        assertEquals(listOf("W", "P1", "draft:c-draft", "P2"), col.items.map { it.key })
    }

    @Test fun placeholderHasNoDetailsAndCannotBeOpenedOrDragged() {
        val item = proposals(draftChanges(drafting.copy(changeName = "secret-name"))).items.single { it is LaneItem.DraftPlaceholder }
        assertNull(item.title)
        assertNull(item.changeName)
        assertFalse(item.clickable)
        assertFalse(item.draggable)
        assertFalse("secret-name" in item.toString() || "t D1" in item.toString(), "the item carries no title or change name")
        val card = proposals(draftChanges(drafting)).items.first { it is LaneItem.Card }
        assertTrue(card.clickable, "ordinary proposal cards still open")
        assertFalse(card.draggable, "proposals move by approval, not by dragging")
    }

    @Test fun createdProposalTakesThePlaceholdersPositionWithOthersUnmoved() {
        val before = proposals(draftChanges(drafting)).items.map { it.key }
        // Created: created_at set, the draft gone, a later update time than every other proposal.
        val created = drafting.copy(phase = "reviewing", changeName = "drafts", draft = null, createdAt = "2026-11-01T05:00:00Z", updatedAt = "2026-11-01T05:00:00Z")
        val col = proposals(draftChanges(created))
        val after = col.items.map { it.key }
        assertEquals(before.indexOf("draft:c-draft"), after.indexOf("D1"), "the real card is where the placeholder was")
        assertEquals(before.filter { it != "draft:c-draft" }, after.filter { it != "D1" }, "other items haven't moved")
        assertEquals(1, after.count { it == "D1" || it == "draft:c-draft" }, "exactly one item for that change")
        assertIs<LaneItem.Card>(col.items[after.indexOf("D1")])
    }

    @Test fun revisionOfACreatedChangeIsAnOrdinaryCard() {
        val revising = drafting.copy(changeName = "drafts", createdAt = "2026-10-01T00:00:00Z", draft = null)
        val col = proposals(draftChanges(revising))
        assertTrue(col.items.none { it is LaneItem.DraftPlaceholder })
        val card = assertIs<LaneItem.Card>(col.items.single { it.key == "D1" })
        assertEquals("t D1", card.title)
        assertTrue(card.clickable)
    }

    @Test fun olderBridgeWithoutStartedAtKeepsTodaysUpdatedAtOrderAndLane() {
        // No started_at, created_at or draft: exactly the lane the first test pins down.
        val col = board.columns(GroupBy.WORKFLOW, BoardFilter(), changes = changes).first()
        assertEquals(listOf("S2", "S1", "A", "S3"), col.items.map { it.key })
        assertEquals(col.issues.map { it.id }, col.items.map { it.key })
        assertTrue(col.items.all { it is LaneItem.Card })
        assertEquals(4, col.total)
        // And a decoded summary without the new fields has no draft and sorts by updated_at.
        val decoded = kotlinx.serialization.json.Json { ignoreUnknownKeys = true }.decodeFromString<ChangesList>(
            """{"openspec":true,"total":2,"changes":[
                {"id":"a","phase":"drafting","seed_ids":["P1"],"updated_at":"2026-11-01T01:00:00Z"},
                {"id":"b","phase":"drafting","seed_ids":["P2"],"updated_at":"2026-11-01T02:00:00Z"}]}""",
        )
        assertNull(decoded.changes.first().draft)
        assertEquals("", decoded.changes.first().startedAt)
        assertEquals(listOf("P2", "P1"), proposals(decoded).items.map { it.key })
    }

    @Test fun newFieldsAreParsed() {
        val decoded = kotlinx.serialization.json.Json { ignoreUnknownKeys = true }.decodeFromString<ChangesList>(
            """{"openspec":true,"changes":[{"id":"a","phase":"needs_attention","seed_ids":["P1"],"updated_at":"u",
                "created_at":"","started_at":"2026-11-01T01:00:00Z","draft":{"kind":"failed","message":"author error"}}]}""",
        )
        val c = decoded.changes.single()
        assertEquals("2026-11-01T01:00:00Z", c.startedAt)
        assertEquals(ChangeDraft("failed", "author error"), c.draft)
        assertTrue(c.shownAsDraft)
        val full = kotlinx.serialization.json.Json { ignoreUnknownKeys = true }.decodeFromString<com.ryderdavid.nacre.data.model.SpecChange>(
            """{"id":"a","seed_id":"P1","phase":"drafting","created_at":"c","started_at":"s","draft":{"kind":"pending"}}""",
        )
        assertEquals("c", full.createdAt)
        assertEquals("s", full.startedAt)
        assertTrue(full.draft!!.pending)
    }

    @Test fun eventForAnUnknownChangeRefreshesTheSummary() {
        val listed = draftChanges(drafting)
        assertTrue(changeEventNeedsRefresh(listed, "c-unknown", "drafting"), "a change not in the summary is fetched, not slotted by guess")
        assertTrue(changeEventNeedsRefresh(null, "c-unknown", "drafting"))
        assertTrue(changeEventNeedsRefresh(listed, "c-draft", "reviewing"), "a phase the summary doesn't show yet")
        assertFalse(changeEventNeedsRefresh(listed, "c-draft", "drafting"), "already reflected")
        // The lane itself never invents an item for it.
        assertTrue(proposals(listed).items.none { it.key.contains("c-unknown") })
    }
}
