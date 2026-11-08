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

    @Test
    fun `a found card's ask is sent beside the option and an empty answer takes its default`() {
        val ask = DecisionInput(kind = "choice", label = "Found in which round bead?", required = true,
            choices = listOf(com.ryderdavid.nacre.data.model.DecisionChoice("N-3", "N-3"), com.ryderdavid.nacre.data.model.DecisionChoice("N-4", "N-4")), default = "N-3")
        val d = Decision(id = "d1", kind = "found", options = listOf(DecisionOption("how_order", "Changes how or order")), ask = ask)
        val draft = DecisionDraft(option = "how_order")
        assertNull(DecisionModel.problem(d, draft))
        assertNull(DecisionModel.answer(d, draft).ask)
        assertEquals("N-4", DecisionModel.answer(d, draft.copy(ask = "N-4")).ask)
        val noDefault = d.copy(ask = ask.copy(default = ""))
        assertEquals("Pick one", DecisionModel.problem(noDefault, draft))
    }

    @Test
    fun `a disabled option is never preselected or sent`() {
        val d = Decision(id = "i1", kind = "intake", options = listOf(
            DecisionOption("amend:c1", "Amends c1", recommended = true, disabled = "Round r1 is working on c1"),
            DecisionOption("later", "Create the bead, develop later"),
        ))
        assertEquals("later", DecisionModel.initialOption(d))
        assertEquals("Round r1 is working on c1", DecisionModel.problem(d, DecisionDraft(option = "amend:c1")))
        assertNull(DecisionModel.problem(d, DecisionDraft(option = "later")))
    }

    @Test
    fun `a chat's expired intake or found card isn't offered Ask again`() {
        assertFalse(DecisionModel.canRenew(Decision(id = "i", kind = "intake", source = "agent", state = "expired")))
        assertFalse(DecisionModel.canRenew(Decision(id = "f", kind = "found", source = "agent", state = "expired")))
        assertTrue(DecisionModel.canRenew(Decision(id = "a", kind = "approve", source = "agent", state = "expired")))
    }

    @Test
    fun `a chat's expired intake or found card is asked again from its chat`() {
        val intake = Decision(id = "i", kind = "intake", source = "agent", state = "expired", chatId = "ch_1",
            consequences = listOf("Intake says: New change", "Intake cost $0.04; nothing develops until you choose"))
        val found = Decision(id = "f", kind = "found", source = "agent", state = "expired", chatId = "ch_1")
        assertTrue(DecisionModel.canAskChat(intake))
        assertTrue(DecisionModel.canAskChat(found))
        // Not while it still waits, not for what nacred raises again itself, and not without a chat to ask.
        assertFalse(DecisionModel.canAskChat(intake.copy(state = "pending")))
        assertFalse(DecisionModel.canAskChat(Decision(id = "a", kind = "approve", source = "agent", state = "expired", chatId = "ch_1")))
        assertFalse(DecisionModel.canAskChat(found.copy(chatId = null)))
        assertFalse(DecisionModel.canAskChat(found.copy(chatId = "")))
        // Raising intake again is a paid call, so the message Ryder sends says what it cost last time.
        assertEquals(
            "This card expired before I answered it. Please raise it again so I can decide. That runs intake again, which cost $0.04 last time.",
            DecisionModel.askChatText(intake),
        )
        assertTrue(DecisionModel.askChatText(intake.copy(consequences = emptyList())).endsWith("That runs intake again, which is a paid call."))
        assertEquals("This card expired before I answered it. Please raise it again so I can decide.", DecisionModel.askChatText(found))
    }

    @Test
    fun `the main button says the option it sends`() {
        val one = Decision(id = "dc_9", kind = "recovery", options = listOf(DecisionOption("resume", "Resume")))
        assertEquals("Resume", DecisionModel.singleOption(one)?.label)
        assertEquals("Resume", DecisionModel.primaryLabel(one, DecisionDraft(option = "resume")))
        assertEquals("Retry with the failure", DecisionModel.primaryLabel(recovery, DecisionDraft(option = "retry")))
        // An option with an input still needs the list; a long label falls back to the kind's verb.
        assertNull(DecisionModel.singleOption(recovery.copy(options = recovery.options.drop(1))))
        val long = recovery.copy(options = listOf(DecisionOption("x", "Keep the current limit and stop here for now")))
        assertEquals("Keep the current limit and stop here for now", DecisionModel.primaryLabel(long, DecisionDraft(option = "x")))
        assertEquals("Choose", DecisionModel.primaryLabel(recovery.copy(options = recovery.options + long.options), DecisionDraft(option = "x")))
        assertEquals("Approve", DecisionModel.primaryLabel(approve, DecisionDraft(option = "run_hold")))
        assertEquals("Reject", DecisionModel.primaryLabel(approve, DecisionDraft(rejecting = true)))
    }

    @Test
    fun `cards the check closed are listed newest first for a week, and leave the queue`() {
        val now = java.time.Instant.parse("2026-11-08T12:00:00Z").toEpochMilli()
        fun closed(id: String, at: String) = Decision(id = id, kind = "found", source = "attention", state = "expired", acknowledged = true,
            handled = com.ryderdavid.nacre.data.model.DecisionHandled("NAC-1 fixed it", listOf("NAC-1"), at))
        val old = closed("old", "2026-10-30T12:00:00Z")
        val a = closed("a", "2026-11-08T10:00:00Z")
        val b = closed("b", "2026-11-08T11:00:00Z")
        val expired = Decision(id = "x", kind = "found", source = "attention", state = "expired")
        val pending = Decision(id = "p", kind = "question", state = "pending")
        val all = listOf(old, a, b, expired, pending)
        assertEquals(listOf("b", "a"), DecisionModel.handled(all, now).map { it.id })
        assertEquals(listOf("p"), DecisionModel.queue(all).map { it.id })
    }

    @Test
    fun `the queue says what its card check is doing`() {
        val c = com.ryderdavid.nacre.data.model.CardCheck(checking = 3)
        assertEquals("Checking 3 cards for anything already handled elsewhere…", com.ryderdavid.nacre.ui.workspace.checkLine(c.copy(running = true)))
        assertNull(com.ryderdavid.nacre.ui.workspace.checkLine(com.ryderdavid.nacre.data.model.CardCheck()))
        assertEquals("Checking the cards for anything already handled elsewhere…", com.ryderdavid.nacre.ui.workspace.checkLine(com.ryderdavid.nacre.data.model.CardCheck(running = true)))
        assertEquals("Closed 2 already handled elsewhere; they're listed at the end",
            com.ryderdavid.nacre.ui.workspace.checkLine(c.copy(finishedAt = "t", handled = listOf("a", "b"))))
        assertEquals("Checked 3 cards: they all still need you", com.ryderdavid.nacre.ui.workspace.checkLine(c.copy(finishedAt = "t")))
        assertTrue(com.ryderdavid.nacre.ui.workspace.checkLine(c.copy(finishedAt = "t", error = "timed out"))!!.endsWith("timed out"))
    }

    @Test
    fun `Projects says how many projects it is checking, then how many cards it closed`() {
        val c = com.ryderdavid.nacre.data.model.CardCheck(checking = 2)
        val line: (List<com.ryderdavid.nacre.data.model.CardCheck>) -> String? = { com.ryderdavid.nacre.ui.workspace.projectsCheckLine(it) }
        assertNull(line(emptyList()))
        assertEquals("Checking 2 projects for cards already handled elsewhere…", line(listOf(c.copy(running = true), c.copy(running = true), c.copy(finishedAt = "t", handled = listOf("a")))))
        assertEquals("Closed 3 cards already handled elsewhere", line(listOf(c.copy(finishedAt = "t", handled = listOf("a")), c.copy(finishedAt = "t", handled = listOf("b", "c")))))
        assertEquals("Checked 1 project: everything still needs you", line(listOf(c.copy(finishedAt = "t"))))
        assertNull(line(listOf(c.copy(finishedAt = "t", checking = 0))))
    }

    @Test
    fun `the walkthrough follows the check's order, then the rest, and only cards that wait on Ryder`() {
        val q = Decision(id = "q", kind = "question", state = "pending", createdAt = "2026-11-08T10:00:00Z")
        val f = Decision(id = "f", kind = "found", state = "pending", createdAt = "2026-11-08T11:00:00Z")
        val a = Decision(id = "a", kind = "approve", state = "held", createdAt = "2026-11-08T09:00:00Z")
        val ap = Decision(id = "ap", kind = "approve", state = "pending", createdAt = "2026-11-08T08:00:00Z")
        val run = Decision(id = "r", kind = "found", state = "executing")
        val later = Decision(id = "d", kind = "question", state = "deferred")
        val order = listOf(
            com.ryderdavid.nacre.data.model.CardStep("a", "Round 3 waits on it"),
            com.ryderdavid.nacre.data.model.CardStep("r", "running, so dropped"),
            com.ryderdavid.nacre.data.model.CardStep("gone", "unknown, so dropped"),
            com.ryderdavid.nacre.data.model.CardStep("q", "Decides how f goes"),
            com.ryderdavid.nacre.data.model.CardStep("a", "twice"),
        )
        val steps = DecisionModel.walkSteps(listOf(q, f, a, ap, run, later), order)
        assertEquals(listOf("a", "q", "f", "ap"), steps.map { it.first.id })
        assertEquals(listOf("Round 3 waits on it", "Decides how f goes", "", ""), steps.map { it.second })
        // With no order yet, it's the queue's order.
        assertEquals(listOf("f", "q", "ap", "a"), DecisionModel.walkSteps(listOf(q, f, a, ap), emptyList()).map { it.first.id })
    }

    @Test
    fun `the walkthrough says where it is and how it went`() {
        assertEquals("Card 2 of 5", walkHeading(1, 5))
        assertEquals("You answered all 4.", walkSummary(4, 0))
        assertEquals("You skipped all 2; they're still in the list.", walkSummary(0, 2))
        assertEquals("Answered 3, skipped 1. The skipped ones are still in the list.", walkSummary(3, 1))
        assertEquals("Walk me through the 4, in order", com.ryderdavid.nacre.ui.workspace.walkButton(4))
    }
}
