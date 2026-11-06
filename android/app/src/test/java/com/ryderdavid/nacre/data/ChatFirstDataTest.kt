package com.ryderdavid.nacre.data

import com.ryderdavid.nacre.data.model.AttentionItem
import com.ryderdavid.nacre.data.model.AttentionOption
import com.ryderdavid.nacre.data.model.BEAD_INTEGRATED
import com.ryderdavid.nacre.data.model.BEAD_PARKED
import com.ryderdavid.nacre.data.model.ROUND_IMPLEMENTING
import com.ryderdavid.nacre.data.model.ROUND_PAUSED
import com.ryderdavid.nacre.data.model.ROUND_READY
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.RoundBead
import com.ryderdavid.nacre.data.model.WorkSummary
import com.ryderdavid.nacre.ui.workspace.stripWork
import com.ryderdavid.nacre.ui.workspace.work.CANCEL_ROUND_TEXT
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ChatFirstDataTest {
    private fun round(state: String) = Round(
        "rd1", repoId = "r1", state = state, spentUsd = 6.4, budgetUsd = 50.0,
        beads = listOf(RoundBead("a", state = BEAD_INTEGRATED), RoundBead("b", state = BEAD_PARKED), RoundBead("c")),
    )

    @Test
    fun `a round becomes a work summary with landed of total`() {
        val w = ChatFirstData.work(round(ROUND_IMPLEMENTING))
        assertEquals("running", w.state)
        assertEquals(1, w.landed)
        assertEquals(3, w.total)
        assertEquals("Round running · 1 of 3 landed", ChatFirstData.statusText(w))
    }

    @Test
    fun `a paused round is stopped, a ready one is ready to land`() {
        assertEquals("Round stopped · 1 of 3 landed", ChatFirstData.statusText(ChatFirstData.work(round(ROUND_PAUSED))))
        assertEquals("Ready to land · 1 of 3", ChatFirstData.statusText(ChatFirstData.work(round(ROUND_READY))))
    }

    @Test
    fun `the strip shows stopped work before running work`() {
        val w = stripWork(listOf(WorkSummary(id = "a", state = "running"), WorkSummary(id = "b", state = "stopped"), WorkSummary(id = "c", state = "done")))
        assertEquals("b", w?.id)
    }

    @Test
    fun `an attention item becomes a decision whose options perform its own actions`() {
        val item = AttentionItem(
            "q1", AttentionItem.ROUND_QUESTION, "r1", title = "Where should state live?", why = "Asked by NAC-axe.2",
            options = listOf(AttentionOption("action", "With each chat", recommended = true), AttentionOption("open", "Open the round"), AttentionOption("action", "Shared", disabled = "Round busy")),
            hostId = "h1",
        )
        val d = ChatFirstData.decision(item)
        assertEquals("question", d.kind)
        assertEquals("att_q1", d.id)
        assertEquals(listOf("With each chat"), d.options.map { it.label })
        assertTrue(d.options.single().recommended)
    }

    @Test
    fun `cancel says what keeps running`() {
        assertTrue(CANCEL_ROUND_TEXT.contains("Chats keep running"))
    }
}
