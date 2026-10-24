package com.ryderdavid.nacre.data.model

import com.ryderdavid.nacre.data.bridge.BridgeClient
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The adopted-run contract from docs/API.md: the extra fields are absent on a run the Mac
 * dispatched, and an absent capability flag means "allowed".
 */
class AdoptedRunTest {
    private fun decode(json: String) = BridgeClient.json.decodeFromString<Run>(json)

    @Test fun aRunFromAnOlderBridgeIsOursAndKeepsEveryControl() {
        val r = decode("""{"id":"r_1","status":"running","session_id":"s1","resume_command":"cd x && claude --resume s1"}""")
        assertFalse(r.isAdopted)
        assertTrue(r.isNacreWork)
        assertTrue(r.showsCancel)
        assertNull(r.canCancel, "absence is the signal: the bridge only sends the flag to refuse")
        assertEquals("cd x && claude --resume s1", r.takeoverCommand)
    }

    @Test fun anAdoptedRunOffersNeitherCancelNorFollowUp() {
        val running = decode(
            """{"id":"r_2","status":"running","origin":"adopted","agent":"claude-code","session_name":"fix-parser",
               "owner_pid":54321,"activity":"working","can_cancel":false,"can_followup":false,"session_id":"s2"}"""
        )
        assertTrue(running.isAdopted)
        assertFalse(running.showsCancel)
        assertFalse(running.showsFollowUp)
        assertEquals("fix-parser", running.sessionName)
        assertEquals(54321, running.ownerPid)

        val ended = running.copy(status = "succeeded", activity = ACTIVITY_ENDED)
        assertFalse(ended.showsFollowUp, "a session that ended is still not ours to resume")
        assertFalse(ended.showsCancel)
    }

    @Test fun anAdoptedRunIsNeverWorkNacreIsWaitingOn() {
        val adopted = Run(id = "r_3", status = "running", origin = ORIGIN_ADOPTED, activity = ACTIVITY_IDLE)
        assertTrue(adopted.isActive, "it is live work, and the Runs tab says so")
        assertFalse(adopted.isNacreWork, "but nobody asked for it from here")
    }

    @Test fun ourOwnFinishedRunStillTakesAFollowUp() {
        val mine = Run(id = "r_4", status = "succeeded", sessionId = "s4")
        assertTrue(mine.showsFollowUp)
        assertFalse(mine.showsCancel, "nothing to cancel once it is over")
        assertFalse(Run(id = "r_5", status = "succeeded").showsFollowUp, "no session to resume")
    }

    @Test fun aStaleAgentSessionStaysOnItsBeadButLeavesTheRunsList() {
        val stale = decode(
            """{"id":"r_6","status":"running","origin":"adopted","activity":"idle","stale":true,
               "issue_id":"NAC-7","last_activity_at":"2026-10-23T19:53:01.123Z"}"""
        )
        assertTrue(stale.isActive, "still a live session, so its bead keeps showing it, as idle")
        assertEquals(ACTIVITY_IDLE, stale.activity)
        assertFalse(stale.inRunsList, "a session left open is not a job in progress")
        assertEquals("2026-10-23T19:53:01.123Z", stale.lastActivityAt)
        assertTrue(decode("""{"id":"r_7","status":"running","origin":"adopted","activity":"idle"}""").inRunsList,
            "an idle session that wrote recently is still listed")
        assertTrue(decode("""{"id":"r_8","status":"running"}""").inRunsList)
    }

        @Test fun adoptedRunsAreCountedApartFromOurs() {
        val repo = BridgeClient.json.decodeFromString<Repo>(
            """{"id":"x","name":"Nacre","path":"/tmp/x","active_runs":1,"adopted_runs":2}"""
        )
        assertEquals(1, repo.activeRuns)
        assertEquals(2, repo.adoptedRuns)
        assertEquals(0, BridgeClient.json.decodeFromString<Repo>("""{"id":"x","name":"n","path":"/p"}""").adoptedRuns)
    }
}
