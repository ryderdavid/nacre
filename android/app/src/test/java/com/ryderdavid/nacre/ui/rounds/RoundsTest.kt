package com.ryderdavid.nacre.ui.rounds

import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.model.BEAD_INTEGRATED
import com.ryderdavid.nacre.data.model.BEAD_PARKED
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.ROUND_NEEDS_DECISION
import com.ryderdavid.nacre.data.model.ROUND_READY
import com.ryderdavid.nacre.data.model.Repo
import com.ryderdavid.nacre.data.model.Round
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

class RoundsTest {
    /** A round as nacred sends it (trimmed from a real trial round). */
    private val json = """{"id":"rdtnf6ip77n","repo_id":"r1","repo_name":"calc","state":"ready_to_land","target":"main","base":"c99984f",
        "beads":[{"id":"TRY-mlh","title":"Add a sub command","type":"feature","state":"integrated","fix_attempts":0,"stalls":0,"merge_commit":"44f9034"},
                 {"id":"TRY-b44","title":"List every command","type":"task","state":"parked","after":["TRY-mlh"],"reason":"It still failed","fix_attempts":2,"stalls":0}],
        "plan":{"summary":"All three edit main.go.","beads":[],"findings":[{"id":"f1","kind":"overlap","beads":["TRY-mlh","TRY-b44"],"text":"Both edit main()"}],"cost_usd":0.4},
        "questions":[{"id":"q1","kind":"contradiction","beads":["TRY-mlh"],"text":"Which way?","options":["Left","Right"]}],
        "budget_usd":8,"spent_usd":1.3,"time_limit_minutes":240,"final_commit":"bdfd156","version":"0.1.0 -> 0.2.0",
        "artifacts":["/x/bin/calc"],"some_new_field":true}"""

    @Test fun roundDecodes() {
        val r = BridgeClient.json.decodeFromString<Round>(json).copy(hostId = "h1")
        assertEquals("h1~rdtnf6ip77n", r.key)
        assertEquals(2, r.beads.size)
        assertEquals(listOf("TRY-mlh"), r.beads[1].after)
        assertEquals("overlap", r.plan?.findings?.single()?.kind)
        assertEquals(listOf("Left", "Right"), r.openQuestions.single().options)
        assertTrue(r.needsYou)
        assertTrue(r.isActive)
        assertEquals("1 of 2 merged · 1 parked", beadSummary(r))
        assertEquals("h1~r_9", r.runKey("r_9"))
    }

    @Test fun labelsReadLikeTheApp() {
        assertEquals("Ready to merge", roundStateLabel(ROUND_READY))
        assertEquals("Needs your decision", roundStateLabel(ROUND_NEEDS_DECISION))
        assertEquals("Merged", beadStateLabel(BEAD_INTEGRATED))
        assertEquals("Overlap, run one after the other", findingLabel("overlap"))
        assertEquals("$1.30", usd(1.3))
    }

    @kotlin.test.Test fun automaticLandingAndFinishingDecodeWithoutRequestingAMerge() {
        val round = com.ryderdavid.nacre.data.bridge.BridgeClient.json.decodeFromString<com.ryderdavid.nacre.data.model.Round>(
            """{"id":"r1","state":"ready_to_land","land_when_ready":true,"finish":true}""",
        )
        kotlin.test.assertTrue(round.finish)
        kotlin.test.assertTrue(round.mergesAutomatically)
        kotlin.test.assertFalse(round.needsYou)
        kotlin.test.assertTrue(round.copy(landWhenReady = false, autoLand = true).mergesAutomatically)
        kotlin.test.assertFalse(round.copy(landWhenReady = false).mergesAutomatically)
    }

    // NAC-pjq.8: the finish line names the epic's branch and where it merges, so it can't read as the
    // round's own target.
    @Test fun finishLineNamesTheEpicBranchAndItsTarget() {
        val r = Round(id = "rdtn", epic = "NAC-48p", target = "epic/nac-48p", finish = true, finishTarget = "main")
        assertEquals("Then epic/nac-48p merges into main", finishLine(r))
        assertEquals("Then epic/nac-48p merges into the landing target", finishLine(r.copy(finishTarget = "")))
        assertEquals("Then epic/nac-48p merges into main, once you sign off the checks left", finishLine(r.copy(finishWaiting = true)))
        assertEquals("Finishing: epic/nac-48p merges into main in round rdfin", finishLine(r.copy(finishRoundId = "rdfin")))
        assertEquals("epic/nac-48p wasn't merged into main after landing: see the log", finishLine(r.copy(finishChecked = true)))
        assertNull(finishLine(r.copy(finish = false)))
    }
}
