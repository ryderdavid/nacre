package com.ryderdavid.nacre.data

import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.bridge.HostEvent
import com.ryderdavid.nacre.data.bridge.StreamMessage
import com.ryderdavid.nacre.data.model.Decision
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** Decision notifications: one per decision that newly waits, never for one answered here. */
class DecisionAlertsTest {
    private class Recording : DecisionNotices {
        val posted = mutableListOf<String>()
        val cleared = mutableListOf<String>()
        override fun postDecision(d: Decision) { posted += d.key }
        override fun clearDecision(d: Decision) { cleared += d.key }
    }

    private fun decision(id: String, state: String = "pending") =
        Decision(id = id, repo = "r1", kind = "land", title = "Land round rdto3k", state = state, hostId = "a")

    @Test
    fun `a newly pending decision is announced once, however often it arrives`() {
        val n = Recording()
        val alerts = DecisionAlerts(n, inBackground = { true })
        alerts.seen(decision("dc_1"))
        alerts.seen(decision("dc_1"))
        alerts.seen(decision("dc_2"))
        assertEquals(listOf("a~dc_1", "a~dc_2"), n.posted)
    }

    @Test
    fun `an answered decision clears its notification and is never announced again`() {
        val n = Recording()
        val alerts = DecisionAlerts(n, inBackground = { true })
        alerts.seen(decision("dc_1"))
        alerts.seen(decision("dc_1", "executing"))
        assertEquals(listOf("a~dc_1"), n.cleared)
        // A late "pending" from another source (the chat log, a reconnect) stays quiet.
        alerts.seen(decision("dc_1"))
        assertEquals(listOf("a~dc_1"), n.posted)
    }

    @Test
    fun `a decision Ryder just answered on this device is not announced`() {
        val n = Recording()
        val alerts = DecisionAlerts(n, inBackground = { true })
        // The answer's result is learned before the older "created" event arrives.
        alerts.seen(decision("dc_1", "done"))
        alerts.seen(decision("dc_1"))
        assertTrue(n.posted.isEmpty())
        assertTrue(n.cleared.isEmpty())
    }

    @Test
    fun `nothing is posted in the foreground, and posted once the app is backgrounded`() {
        val n = Recording()
        var background = false
        val alerts = DecisionAlerts(n, inBackground = { background })
        alerts.seen(decision("dc_1"))
        assertTrue(n.posted.isEmpty())
        background = true
        alerts.seen(decision("dc_1"))
        assertEquals(listOf("a~dc_1"), n.posted)
    }

    @Test
    fun `the notification says what is asked and where`() {
        val d = decision("dc_1").copy(subtitle = "Nacre · Mac-Studio · revision 4c0ffee", consequences = listOf("Merges into main and pushes"))
        assertEquals("Land round rdto3k" to "Nacre · Mac-Studio · revision 4c0ffee", DecisionAlerts.text(d))
        assertEquals("A decision needs you" to "Merges into main and pushes", DecisionAlerts.text(d.copy(title = "", subtitle = "")))
    }

    @Test
    fun `events decode decisions, and the run stream's kinds as before`() {
        val ev = BridgeClient.decodeHostEvent(
            "decision",
            """{"id":"dc_8f","repo":"5b08","kind":"approve","state":"failed","acknowledged":true,"deferred_until":null,"target":{"change":"offline-chat-resume","bead":"NAC-axe"},"unknown_field":1}""",
        )
        val d = (ev as HostEvent.DecisionChanged).decision
        assertEquals("dc_8f", d.id)
        assertEquals("offline-chat-resume", d.target.change)
        assertTrue(d.acknowledged)
        assertTrue(!d.open)

        val repo = BridgeClient.decodeHostEvent("repo", """{"repo_id":"5b08"}""")
        assertEquals(HostEvent.Stream(StreamMessage.RepoChanged("5b08")), repo)

        // A chat's log carries the same Decision in its `decision` event.
        val chatEvent = BridgeClient.json.decodeFromString<com.ryderdavid.nacre.data.model.ChatEvent>(
            """{"seq":9,"kind":"decision","turn_id":"t1","decision":{"id":"dc_8f","repo":"5b08","chat_id":"ch_1","kind":"land","state":"pending"}}""",
        )
        val carried = com.ryderdavid.nacre.data.chat.latestDecisions(listOf(chatEvent), "a").single()
        assertEquals("a~dc_8f", carried.key)
        assertEquals("ch_1", carried.chatId)
    }
}
