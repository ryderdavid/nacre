package com.ryderdavid.nacre.data.model

import com.ryderdavid.nacre.data.bridge.BridgeClient
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/** A repo says whether it has nacre.yaml gates, which rounds will need. */
class RepoGatesTest {
    @Test fun gatesDecode() {
        val r = BridgeClient.json.decodeFromString<Repo>(
            """{"id":"r","name":"Nacre","path":"/x","gates":["bridge","app tests"],"gates_error":""}""",
        )
        assertEquals(listOf("bridge", "app tests"), r.gates)
    }

    @Test fun anOlderBridgeMeansNoGates() {
        val r = BridgeClient.json.decodeFromString<Repo>("""{"id":"r","name":"Nacre","path":"/x"}""")
        assertTrue(r.gates.isEmpty())
        assertEquals("", r.gatesError)
    }
}
