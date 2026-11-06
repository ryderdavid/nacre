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
