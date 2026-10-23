package com.ryderdavid.nacre.notify

import com.ryderdavid.nacre.data.model.ACTIVITY_ENDED
import com.ryderdavid.nacre.data.model.ORIGIN_ADOPTED
import com.ryderdavid.nacre.data.model.Run
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/** An agent's own session must not pin a foreground service or claim to have finished. */
class RunNotifierPolicyTest {
    private val ours = Run(id = "r_1", status = "running")
    private val theirs = Run(id = "r_2", status = "running", origin = ORIGIN_ADOPTED)

    @Test fun onlyOurOwnActiveRunsKeepTheWatcherAlive() {
        assertEquals(listOf(ours), runsWorthWatching(listOf(ours, theirs)))
        assertTrue(runsWorthWatching(listOf(theirs)).isEmpty(), "a parked terminal is not work in progress")
        assertTrue(runsWorthWatching(listOf(ours.copy(status = "succeeded"))).isEmpty())
    }

    @Test fun ourRunIsAnnouncedWhenItEndsInTheBackground() {
        val done = ours.copy(status = "succeeded")
        assertTrue(shouldAnnounceFinish(done, notifyRuns = true, appInForeground = false))
        assertFalse(shouldAnnounceFinish(done, notifyRuns = true, appInForeground = true), "the app is showing it already")
        assertFalse(shouldAnnounceFinish(done, notifyRuns = false, appInForeground = false), "notifications are off")
    }

    @Test fun quittingAnAgentsSessionNeverPostsAFinishedNotification() {
        val gone = theirs.copy(status = "succeeded", activity = ACTIVITY_ENDED)
        assertFalse(shouldAnnounceFinish(gone, notifyRuns = true, appInForeground = false))
    }
}
