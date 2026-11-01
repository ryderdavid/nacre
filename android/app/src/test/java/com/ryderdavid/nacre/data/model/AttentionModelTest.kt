package com.ryderdavid.nacre.data.model

import com.ryderdavid.nacre.data.bridge.BridgeClient
import kotlin.test.Test
import kotlin.test.assertEquals

class AttentionModelTest {
    @Test fun decodesTheBridgeContract() {
        val json = """{"items":[{"id":"x","kind":"found_work","repo":"r1","repo_name":"nacre","bead_id":"F-1","change_id":"sf1","title":"t","why":"w",
            "options":[{"type":"action","label":"Revise now","method":"POST","path":"/v1/repos/r1/changes/sf1/found","body":{"answer":"revise","found_beads":["F-1"]},"disabled":"Round R1 is still active."},
            {"type":"open","label":"Make it a follow-on change","target":{"kind":"new_effort","repo":"r1","note":"n","related":["E"],"follow_on_of":"F-1"}}],
            "created_at":"2026-11-01T00:00:00Z","future_field":1}]}"""
        val item = BridgeClient.json.decodeFromString<AttentionList>(json).items.single()
        assertEquals("Round R1 is still active.", item.options[0].disabled)
        assertEquals("F-1", item.options[1].target!!.followOnOf)
        assertEquals(0, item.group)
        assertEquals(1, item.copy(kind = AttentionItem.PROPOSAL_READY).group)
        assertEquals(2, item.copy(kind = AttentionItem.ROUND_READY).group)
        assertEquals("h~r1", item.copy(hostId = "h").repoKey)
    }

    @Test fun changeSummaryPlanning() {
        assertEquals(true, ChangeSummary("c", phase = "awaiting_approval").planning)
        assertEquals(false, ChangeSummary("c", phase = "ready").planning)
        assertEquals(true, ChangeSummary("c", phase = "stale").planning)
        assertEquals(false, ChangeSummary("c", phase = "stale", epicId = "E").planning)
    }
}
