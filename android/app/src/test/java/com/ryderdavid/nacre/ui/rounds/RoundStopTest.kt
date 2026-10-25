package com.ryderdavid.nacre.ui.rounds

import com.ryderdavid.nacre.data.model.CHOICE_RETRY_WITH_BUDGET
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.ROUND_PAUSED
import kotlinx.serialization.json.Json
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class RoundStopTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test fun stopParsesWithDiagnosisChoicesAndSpend() {
        val r = json.decodeFromString<Round>(
            """{"id":"rd1","state":"paused","pause_reason":"main moved","overhead_usd":0.04,
               "stop":{"kind":"merge-conflict","reason":"main moved","phase":"land",
                 "diagnosis":{"summary":"Both sides edit ParseLine.","cause":"transcript.go","cost_usd":0.04},
                 "choices":[{"id":"retry-with-budget","label":"Let Claude try again","explanation":"Another attempt."},{"id":"cancel","label":"Cancel the round"}],
                 "attempted":["turn:land-merge×2"],
                 "spend":[{"bead":"NAC-1","spent_usd":8.1,"done":true}],"estimate_usd":8.1},
               "beads":[{"id":"NAC-2","carried_from":"nacre/r/rd0/NAC-2"}]}""",
        )
        assertEquals(ROUND_PAUSED, r.state)
        val s = r.stop!!
        assertEquals("Both sides edit ParseLine.", s.diagnosis?.summary)
        assertEquals(CHOICE_RETRY_WITH_BUDGET, s.choices.first().id)
        assertEquals(8.1, s.spend.single().spentUsd)
        assertEquals("nacre/r/rd0/NAC-2", r.beads.single().carriedFrom)
        assertEquals(0.04, r.overheadUsd)
    }

    @Test fun olderNacredHasNoStop() {
        assertNull(json.decodeFromString<Round>("""{"id":"rd1","state":"paused","pause_reason":"x"}""").stop)
    }

    @Test fun attemptsReadAsWords() {
        assertEquals("Claude 2 times", attemptLabel("turn:land-merge×2"))
        assertEquals("Claude once", attemptLabel("turn:final×1"))
        assertEquals("settled version without Claude", attemptLabel("fix:version"))
        assertEquals("settled beads jsonl without Claude", attemptLabel("fix:beads-jsonl:.beads/log.jsonl"))
        assertEquals("retried 2×", attemptLabel("retry:2"))
        assertEquals("reviewed 2 times", attemptLabel("review:2"))
    }
}
