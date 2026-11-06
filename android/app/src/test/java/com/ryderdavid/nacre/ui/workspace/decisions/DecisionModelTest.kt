package com.ryderdavid.nacre.ui.workspace.decisions

import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.model.DecisionInput
import com.ryderdavid.nacre.data.model.DecisionOption
import com.ryderdavid.nacre.data.model.DecisionResult
import org.junit.Assert.assertEquals
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
}
