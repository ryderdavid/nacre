package com.ryderdavid.nacre.data.model

import kotlin.test.Test
import kotlin.test.assertEquals

/** The Activity badge counts work going on now; rounds waiting on the user are Inbox's. */
class ActivityBadgeTest {
    private fun round(state: String, autoLand: Boolean = false) = Round(id = "rd_$state", state = state, autoLand = autoLand)

    @Test fun countsRunningRunsAndWorkingRoundsOnly() {
        val runs = listOf(
            Run(id = "r_1", status = "running"),
            Run(id = "r_2", status = "queued"),
            Run(id = "r_3", status = "succeeded"),
            Run(id = "r_4", status = "running", origin = ORIGIN_ADOPTED, stale = true),
        )
        val rounds = listOf(
            round(ROUND_PLANNING),
            round(ROUND_IMPLEMENTING),
            round(ROUND_INTEGRATING),
            round(ROUND_READY, autoLand = true),
            round(ROUND_NEEDS_DECISION),
            round(ROUND_READY),
            round(ROUND_PAUSED),
            round(ROUND_LANDED),
            round(ROUND_CANCELLED),
        )
        assertEquals(2 + 4, activityBadgeCount(runs, rounds))
    }

    @Test fun aRoundWaitingOnADecisionIsNotCounted() {
        assertEquals(0, activityBadgeCount(emptyList(), listOf(round(ROUND_NEEDS_DECISION))))
        assertEquals(1, activityBadgeCount(emptyList(), listOf(round(ROUND_IMPLEMENTING), round(ROUND_NEEDS_DECISION))))
    }
}
