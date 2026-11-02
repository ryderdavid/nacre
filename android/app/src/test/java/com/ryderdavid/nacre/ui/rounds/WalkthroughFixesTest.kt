package com.ryderdavid.nacre.ui.rounds

import com.ryderdavid.nacre.data.board.BoardFilter
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.RoundChanges
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** NAC-3mc: round setup keeps the board's context, and landing says whether it pushes. */
class WalkthroughFixesTest {
    private val index = BoardIndex(
        listOf(
            Issue("E", "Search epic", type = "epic"),
            Issue("A", "Fix the search lane", parent = "E"),
            Issue("B", "Unrelated work"),
            Issue("C", "Search again elsewhere"),
        ),
    )
    private val candidates = roundCandidates(index)

    @Test fun startRoundKeepsTheBoardsSearch() {
        val f = BoardFilter(query = "search")
        assertEquals(listOf("A", "C"), scopedCandidates(candidates, index, f, f.query, keepBoardFilter = true).map { it.id }.sorted())
        // Clearing the sheet's search shows everything again.
        assertEquals(listOf("A", "B", "C"), scopedCandidates(candidates, index, f, "", keepBoardFilter = true).map { it.id }.sorted())
    }

    @Test fun startRoundKeepsTheBoardsEpicUntilTurnedOff() {
        val f = BoardFilter(epic = "E", query = "search")
        assertEquals("under Search epic", boardScopeLabel(index, f))
        assertEquals(listOf("A"), scopedCandidates(candidates, index, f, "search", keepBoardFilter = true).map { it.id })
        assertEquals(listOf("A", "C"), scopedCandidates(candidates, index, f, "search", keepBoardFilter = false).map { it.id }.sorted())
        assertNull(boardScopeLabel(index, BoardFilter(query = "x")))
    }

    @Test fun landingSaysWhetherItPushes() {
        val push = landWording("main", "abc1234", true)
        assertEquals("Fast-forward main and push", push.confirm)
        assertTrue("pushes it to origin" in push.body)
        val local = landWording("main", "abc1234", false)
        assertEquals("Fast-forward main, don't push", local.confirm)
        assertTrue("doesn't push" in local.body && "land.push" in local.body)
        // An older nacred doesn't say; the dialog stays conditional rather than guessing.
        assertTrue("if the repository's nacre.yaml says to" in landWording("main", "abc1234", null).body)
    }

    @Test fun landPushAndChangesDecode() {
        val r = BridgeClient.json.decodeFromString<Round>("""{"id":"r1","state":"ready_to_land","land_push":false}""")
        assertEquals(false, r.landPush)
        assertNull(BridgeClient.json.decodeFromString<Round>("""{"id":"r1"}""").landPush)
        val c = BridgeClient.json.decodeFromString<RoundChanges>(
            """{"base":"b","head":"h","commits":[{"sha":"abc","subject":"Do it"}],
               "files":[{"path":"a.kt","added":3,"deleted":1},{"path":"x.png","added":0,"deleted":0,"binary":true}],"more_commits":2}""",
        )
        assertEquals(3, c.added)
        assertEquals(1, c.deleted)
        assertTrue(c.files[1].binary)
        assertEquals(2, c.moreCommits)
    }
}
