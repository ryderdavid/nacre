package com.ryderdavid.nacre.ui.workspace.decisions

import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.model.DecisionInput
import com.ryderdavid.nacre.data.model.DecisionOption
import com.ryderdavid.nacre.data.model.DecisionResult
import com.ryderdavid.nacre.data.model.DecisionTarget
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class DecisionModelTest {
    private val approve = Decision(
        id = "dc_1", kind = "approve", title = "Approve offline-chat-resume",
        options = listOf(
            DecisionOption("run_hold", "Approve, run, I'll land", "Stops before main", recommended = true),
            DecisionOption("run_land", "Approve, run and land to main", "Merges into main and pushes"),
            DecisionOption("approve_only", "Approve only", "Doesn't start work"),
        ),
        rejectNeedsNote = true,
    )

    @Test
    fun `the recommended option is preselected and distinct options stay distinct`() {
        assertEquals("run_hold", DecisionModel.initialOption(approve))
        assertEquals(3, approve.options.map { it.id }.toSet().size)
        assertEquals("Approve", DecisionModel.sendLabel(approve))
    }

    @Test
    fun `reject on an approval needs a note`() {
        assertEquals("Say why in a note", DecisionModel.problem(approve, DecisionDraft(rejecting = true)))
        assertNull(DecisionModel.problem(approve, DecisionDraft(rejecting = true, note = "Plan misses migrations")))
        val a = DecisionModel.answer(approve, DecisionDraft(rejecting = true, note = " Plan misses migrations "))
        assertEquals(true, a.reject)
        assertEquals("Plan misses migrations", a.note)
    }

    private val recovery = Decision(
        id = "dc_2", kind = "recovery",
        options = listOf(
            DecisionOption("retry", "Retry with the failure", recommended = true),
            DecisionOption("raise", "Raise the limit", input = DecisionInput("amount", min = 6.4, max = 50.0, unit = "usd")),
        ),
    )

    @Test
    fun `a budget amount below what's spent keeps choose disabled and says why`() {
        val p = DecisionModel.problem(recovery, DecisionDraft(option = "raise", input = "5"))
        assertEquals("More than $6.40, which is already spent", p)
        assertEquals("At most $50", DecisionModel.problem(recovery, DecisionDraft(option = "raise", input = "80")))
        assertNull(DecisionModel.problem(recovery, DecisionDraft(option = "raise", input = "$20")))
        assertEquals("20", DecisionModel.answer(recovery, DecisionDraft(option = "raise", input = "$20")).input)
        assertNull(DecisionModel.answer(recovery, DecisionDraft(option = "retry", input = "stale")).input)
    }

    @Test
    fun `approval is never shown as success`() {
        assertEquals("Approved · working on it", DecisionModel.stateLine(approve.copy(state = "executing")))
        assertEquals("Failed: push rejected", DecisionModel.stateLine(approve.copy(state = "failed", result = DecisionResult(false, "push rejected"))))
        assertEquals("Landed at 3f2c1aa", DecisionModel.stateLine(approve.copy(state = "done", result = DecisionResult(true, "Landed at 3f2c1aa"))))
        assertNull(DecisionModel.stateLine(approve))
    }

    @Test
    fun `expired cards can be asked again and held work says why`() {
        val expired = approve.copy(state = "expired", reason = "revision 5 was written")
        assertEquals("Expired: revision 5 was written", DecisionModel.stateLine(expired))
        assertTrue(DecisionModel.canRenew(expired))
        assertEquals("Held by you: wait for the device check", DecisionModel.stateLine(approve.copy(state = "held", reason = "wait for the device check")))
    }

    @Test
    fun `an answered card can't be sent again`() {
        assertNotNull(DecisionModel.problem(approve.copy(state = "done"), DecisionDraft(option = "run_hold")))
    }

    @Test
    fun `the queue puts pending first, then held`() {
        val list = listOf(approve.copy(id = "a", state = "deferred"), approve.copy(id = "b", state = "held"), approve.copy(id = "c"))
        assertEquals(listOf("c", "b", "a"), DecisionModel.queueOrder(list).map { it.id })
    }

    @Test
    fun `a non-finite amount is refused`() {
        assertEquals("Enter an amount", DecisionModel.problem(recovery, DecisionDraft(option = "raise", input = "NaN")))
    }

    @Test
    fun `questions come before approvals, approvals before landing`() {
        val list = listOf(approve.copy(id = "land", kind = "land"), approve.copy(id = "ap"), approve.copy(id = "q", kind = "question"))
        assertEquals(listOf("q", "ap", "land"), DecisionModel.queueOrder(list).map { it.id })
    }

    @Test
    fun `got it takes a failed, unknown or expired outcome out of the queue`() {
        for (state in listOf("failed", "unknown", "expired")) {
            val d = approve.copy(state = state)
            assertTrue(state, d.open)
            assertTrue(state, DecisionModel.canAcknowledge(d))
            val acked = d.copy(acknowledged = true)
            assertFalse(state, acked.open)
            assertFalse(state, DecisionModel.canAcknowledge(acked))
        }
        // Done and superseded aren't listed; a Needs you item that expired no longer waits.
        assertFalse(approve.copy(state = "done").open)
        assertFalse(approve.copy(state = "superseded").open)
        assertFalse(approve.copy(state = "expired", source = "attention").open)
        assertFalse(DecisionModel.canAcknowledge(approve.copy(state = "done")))
    }

    @Test
    fun `needs you counts what waits on Ryder, the rest keeps the queue reachable`() {
        val list = listOf(
            approve.copy(id = "p"), approve.copy(id = "h", state = "held"),
            approve.copy(id = "d", state = "deferred"), approve.copy(id = "f", state = "failed"),
            approve.copy(id = "x", state = "expired", reason = "revision 5 was written"),
            approve.copy(id = "u", state = "unknown", acknowledged = true), approve.copy(id = "fin", state = "done"),
        )
        assertEquals(QueueCount(needsYou = 2, inQueue = 3), DecisionModel.counts(list))
        // Only deferred and unresolved outcomes left: nothing needs him, yet the queue stays reachable.
        val quiet = DecisionModel.counts(list.filter { it.id in setOf("d", "f") })
        assertEquals(0, quiet.needsYou)
        assertTrue(quiet.any)
        assertFalse(DecisionModel.counts(listOf(approve.copy(state = "done"))).any)
    }

    @Test
    fun `the queue lists unresolved outcomes after held work, with expired ones and their reason`() {
        val list = listOf(
            approve.copy(id = "d", state = "deferred"), approve.copy(id = "e", state = "executing"),
            approve.copy(id = "x", state = "expired", reason = "revision 5 was written"), approve.copy(id = "h", state = "held"),
            approve.copy(id = "p"), approve.copy(id = "fin", state = "done"),
        )
        val queue = DecisionModel.queue(list)
        assertEquals(listOf("p", "h", "x", "e", "d"), queue.map { it.id })
        assertEquals("Expired: revision 5 was written", DecisionModel.stateLine(queue[2]))
    }

    @Test
    fun `the queue opened at a decision lists it first, even once it's answered`() {
        val list = listOf(approve.copy(id = "p"), approve.copy(id = "h", state = "held"), approve.copy(id = "fin", state = "done", result = DecisionResult(true, "Landed at 3f2c1aa")))
        assertEquals(listOf("h", "p"), DecisionModel.queue(list, focus = "h").map { it.id })
        assertEquals(listOf("fin", "p", "h"), DecisionModel.queue(list, focus = "fin").map { it.id })
        assertEquals(listOf("p", "h"), DecisionModel.queue(list, focus = "gone").map { it.id })
    }

    @Test
    fun `a change waiting for Ryder finds its decision by change, name or seed`() {
        val list = listOf(
            approve.copy(id = "old", state = "superseded", target = DecisionTarget(change = "offline-chat-resume")),
            approve.copy(id = "land", kind = "land", target = DecisionTarget(change = "offline-chat-resume", round = "rd1"), createdAt = "2026-11-06T02:00:00Z"),
            approve.copy(id = "ap", target = DecisionTarget(bead = "NAC-axe"), createdAt = "2026-11-06T01:00:00Z"),
            approve.copy(id = "other", target = DecisionTarget(change = "set-up-gates")),
        )
        assertEquals("ap", DecisionModel.forChange(list, "ch_1", "offline-chat-resume", setOf("NAC-axe"))?.id)
        assertEquals("land", DecisionModel.forChange(list, "offline-chat-resume", "", emptySet())?.id)
        assertNull(DecisionModel.forChange(list, "ch_9", "nothing", setOf("NAC-zzz")))
    }

    @Test
    fun `the recommended option is shown first`() {
        val d = approve.copy(options = approve.options.reversed())
        assertEquals("run_hold", DecisionModel.ordered(d).first().id)
    }
}
