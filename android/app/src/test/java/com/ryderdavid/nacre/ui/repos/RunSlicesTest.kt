package com.ryderdavid.nacre.ui.repos

import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Repo
import kotlin.test.Test
import kotlin.test.assertEquals

class RunSlicesTest {
    private val issues = listOf(
        Issue("A-1", status = "in_progress"),
        Issue("A-2", status = "open"),
        Issue("A-3", status = "in_progress"),
        Issue("A-4", status = "open"),
        Issue("A-5", status = "closed"),
    )
    private val counts = Repo.Counts(open = 2, inProgress = 2, closed = 1, total = 5)

    @Test fun aBoardWithNoLiveRunLightsNothing() {
        assertEquals(emptyMap(), runSlices(counts, emptySet(), issues))
    }

    @Test fun aRunningBeadLightsItsOwnSliceOfItsStatusSegment() {
        assertEquals(mapOf(BAR_DOING to setOf(1)), runSlices(counts, setOf("A-3"), issues))
        assertEquals(mapOf(BAR_READY to setOf(1)), runSlices(counts, setOf("A-4"), issues), "a run on an open bead lights the ready segment")
    }

    @Test fun everyRunningBeadLightsItsSlice() {
        assertEquals(mapOf(BAR_DOING to setOf(0, 1), BAR_READY to setOf(0)), runSlices(counts, setOf("A-1", "A-2", "A-3"), issues))
    }

    @Test fun beforeTheBoardLoadsRunningBeadsAreTakenAsInProgress() {
        assertEquals(mapOf(BAR_DOING to setOf(0, 1)), runSlices(counts, setOf("A-9", "A-8"), null))
    }

    @Test fun aSliceTheCountsHaveNoRoomForIsDropped() {
        assertEquals(emptyMap(), runSlices(Repo.Counts(open = 1, total = 1), setOf("A-1"), null), "no doing segment to light")
    }

    @Test fun statusesMapToTheBarsSegments() {
        assertEquals(listOf(BAR_DOING, BAR_DOING, BAR_READY, BAR_BLOCKED, BAR_LATER, BAR_LATER, BAR_DONE), listOf("in_progress", "hooked", "open", "blocked", "deferred", "pinned", "closed").map(::barSegment))
    }
}
