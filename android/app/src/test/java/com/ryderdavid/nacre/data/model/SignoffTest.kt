package com.ryderdavid.nacre.data.model

import kotlinx.serialization.json.Json
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/** A bead whose only remaining work is Ryder checking it, told apart from one that stopped. */
class SignoffTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test fun aBeadLeftForYourCheckSaysWhatToCheck() {
        val i = json.decodeFromString(Issue.serializer(),
            """{"id":"NAC-1","status":"open","labels":["signoff"],"metadata":{"signoff":"Check the spinner on a device."}}""")
        assertTrue(i.awaitsSignoff)
        assertTrue(i.needsYou)
        assertEquals("Check the spinner on a device.", i.signoffCheck)
    }

    @Test fun anOrdinaryOrClosedBeadDoesNotAwaitSignoff() {
        assertFalse(json.decodeFromString(Issue.serializer(), """{"id":"NAC-2","status":"open"}""").awaitsSignoff)
        assertFalse(Issue(id = "NAC-3", status = "closed", labels = listOf("signoff")).awaitsSignoff)
        assertEquals("", Issue(id = "NAC-4", labels = listOf("signoff")).signoffCheck)
    }
}
