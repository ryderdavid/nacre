package com.ryderdavid.nacre.notify

import com.ryderdavid.nacre.data.model.BEAD_PARKED
import com.ryderdavid.nacre.data.model.BEAD_RUNNING
import com.ryderdavid.nacre.data.model.Question
import com.ryderdavid.nacre.data.model.ROUND_IMPLEMENTING
import com.ryderdavid.nacre.data.model.ROUND_LANDED
import com.ryderdavid.nacre.data.model.ROUND_NEEDS_DECISION
import com.ryderdavid.nacre.data.model.ROUND_PLANNING
import com.ryderdavid.nacre.data.model.ROUND_READY
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.RoundBead
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** A round tells Ryder once about each thing that needs him, and stays quiet otherwise. */
class RoundNoticeTest {
    private val base = Round(id = "rd1", state = ROUND_PLANNING, target = "main", beads = listOf(RoundBead("NAC-1", state = BEAD_RUNNING), RoundBead("NAC-2", state = BEAD_RUNNING)))

    @Test fun decisionsAreLeftToTheNeedsYouSummary() {
        // A question and a round ready to land are decisions: the "N things need you" summary announces them.
        val after = base.copy(state = ROUND_NEEDS_DECISION, questions = listOf(Question("q1", text = "Hide the bar on scroll?")))
        assertNull(roundNotice(base, after))
        assertNull(roundNotice(base, base.copy(state = ROUND_READY)))
    }

    @Test fun landed() {
        val ready = base.copy(state = ROUND_READY)
        val landed = ready.copy(state = ROUND_LANDED, landedCommit = "abcdef123", pushed = true)
        assertEquals("main is at abcdef1 and pushed.", roundNotice(ready, landed)?.body)
    }

    @Test fun parkedBeadOnce() {
        val working = base.copy(state = ROUND_IMPLEMENTING)
        val parked = working.copy(beads = listOf(RoundBead("NAC-1", state = BEAD_PARKED, reason = "gate failed"), base.beads[1]))
        assertEquals("NAC-1 was parked", roundNotice(working, parked)?.title)
        assertNull(roundNotice(parked, parked))
    }

    @Test fun quietOtherwise() {
        assertNull(roundNotice(base, base.copy(spentUsd = 2.0)))
    }
}
