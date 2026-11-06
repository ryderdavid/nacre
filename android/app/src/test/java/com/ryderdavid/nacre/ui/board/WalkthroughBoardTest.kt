package com.ryderdavid.nacre.ui.board

import com.ryderdavid.nacre.data.board.Column
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** NAC-3mc: cover-screen search and the new bead sheet's hand-written fields. */
class WalkthroughBoardTest {
    private fun col(key: String, total: Int) = Column(key, key, emptyList(), total)

    @Test fun searchMovesToTheNearestLaneWithMatches() {
        val cols = listOf(col("proposed", 0), col("ready", 0), col("doing", 1), col("blocked", 0), col("done", 4))
        assertEquals(2, laneWithMatches(cols, 1))
        assertEquals(2, laneWithMatches(cols, 0))
        assertEquals(2, laneWithMatches(cols, 3), "ties go to the earlier lane")
        assertNull(laneWithMatches(cols, 2), "a lane with matches stays put")
        assertNull(laneWithMatches(cols.map { col(it.key, 0) }, 1), "nothing matches anywhere")
    }

}
