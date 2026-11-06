package com.ryderdavid.nacre.data

import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.notify.decisionCheckDue
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** The background decision check: which decisions are new, and never twice with the live stream. */
class AnnouncedDecisionsTest {
    private fun decision(id: String, state: String = "pending", host: String = "a") =
        Decision(id = id, repo = "r1", kind = "land", title = "Land round rdto3k", state = state, hostId = host)

    private class Recording : DecisionNotices {
        val posted = mutableListOf<String>()
        val cleared = mutableListOf<String>()
        override fun postDecision(d: Decision) { posted += d.key }
        override fun clearDecision(d: Decision) { cleared += d.key }
    }

    /** A ledger whose first check has already run. */
    private fun checked() = AnnouncedDecisions(AnnouncedDecisions.Memory().apply { checked = true })

    @Test
    fun `only pending decisions not yet announced are new, once each`() {
        val open = listOf(
            decision("dc_1"),
            decision("dc_2", "held"),
            decision("dc_3", "deferred"),
            decision("dc_4"),
            decision("dc_4"),
            decision("dc_1", host = "b"),
        )
        val fresh = AnnouncedDecisions.newDecisions(open, announced = setOf("a~dc_4"))
        assertEquals(listOf("a~dc_1", "b~dc_1"), fresh.map { it.key })
    }

    @Test
    fun `a check announces a new decision once, then never again`() {
        val ledger = checked()
        assertEquals(listOf("a~dc_1"), ledger.claim(listOf(decision("dc_1"))).map { it.key })
        assertTrue(ledger.claim(listOf(decision("dc_1"))).isEmpty())
        assertEquals(listOf("a~dc_2"), ledger.claim(listOf(decision("dc_1"), decision("dc_2"))).map { it.key })
    }

    @Test
    fun `the first check ever only records what already waits`() {
        val ledger = AnnouncedDecisions()
        assertTrue(ledger.claim(listOf(decision("dc_1"), decision("dc_2"))).isEmpty())
        assertTrue(ledger.contains("a~dc_1"))
        assertEquals(listOf("a~dc_3"), ledger.claim(listOf(decision("dc_1"), decision("dc_3"))).map { it.key })
    }

    @Test
    fun `a decision the live stream announced is not announced again by the check`() {
        val ledger = checked()
        val n = Recording()
        DecisionAlerts(n, inBackground = { true }, announced = ledger).seen(decision("dc_1"))
        assertEquals(listOf("a~dc_1"), n.posted)
        assertTrue(ledger.claim(listOf(decision("dc_1"))).isEmpty())
    }

    @Test
    fun `the live stream neither repeats a checked decision nor leaves its notification once answered`() {
        val ledger = checked()
        ledger.claim(listOf(decision("dc_1")))
        // A new process: the stream's own memory is empty, the ledger isn't.
        val n = Recording()
        val alerts = DecisionAlerts(n, inBackground = { true }, announced = ledger)
        alerts.seen(decision("dc_1"))
        assertTrue(n.posted.isEmpty())
        alerts.seen(decision("dc_1", "done"))
        assertEquals(listOf("a~dc_1"), n.cleared)
    }

    @Test
    fun `the ledger keeps the newest keys`() {
        assertEquals(listOf("b", "c", "a"), AnnouncedDecisions.remember(listOf("a", "b", "c"), listOf("a"), cap = 3))
        assertEquals(listOf("c", "d"), AnnouncedDecisions.remember(listOf("a", "b", "c"), listOf("d"), cap = 2))
        assertEquals(listOf("a"), AnnouncedDecisions.remember(listOf("a"), emptyList()))
    }

    @Test
    fun `the check runs only with notifications on, a computer paired and no stream open`() {
        assertTrue(decisionCheckDue(notifyRuns = true, computers = 1, streamOpen = false))
        assertFalse(decisionCheckDue(notifyRuns = false, computers = 1, streamOpen = false))
        assertFalse(decisionCheckDue(notifyRuns = true, computers = 0, streamOpen = false))
        assertFalse(decisionCheckDue(notifyRuns = true, computers = 2, streamOpen = true))
    }
}
