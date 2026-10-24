package com.ryderdavid.nacre.notify

import com.ryderdavid.nacre.data.model.Run
import com.ryderdavid.nacre.data.bridge.BridgeClient
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/** A run the Mac flags as stalled is announced once, and only if Nacre started it. */
class StalledRunTest {
    private fun run(id: String, stalled: Boolean, origin: String = "") =
        Run(id = id, status = "running", stalled = stalled, origin = origin, hostId = "h")

    @Test fun eachStallIsAnnouncedOnce() {
        val runs = listOf(run("a", stalled = true), run("b", stalled = false))
        assertEquals(listOf("a"), newlyStalled(runs, emptySet()).map { it.id })
        assertTrue(newlyStalled(runs, setOf(runs[0].key)).isEmpty())
    }

    @Test fun anAgentsOwnSessionIsNeverAnnounced() {
        assertTrue(newlyStalled(listOf(run("c", stalled = true, origin = "adopted")), emptySet()).isEmpty())
    }

    @Test fun theBridgeFlagDecodes() {
        val r = BridgeClient.json.decodeFromString<Run>(
            """{"id":"r1","status":"running","stalled":true,"stalled_since":"2026-10-24T03:12:53Z"}""",
        )
        assertTrue(r.stalled)
        assertEquals("2026-10-24T03:12:53Z", r.stalledSince)
    }
}
