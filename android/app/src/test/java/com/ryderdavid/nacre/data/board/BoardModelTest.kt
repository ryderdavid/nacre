package com.ryderdavid.nacre.data.board

import com.ryderdavid.nacre.data.model.Dependency
import com.ryderdavid.nacre.data.model.Issue
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class BoardModelTest {
    private fun issue(id: String, status: String = "open", p: Int = 2, type: String = "task", parent: String = "", blocks: List<String> = emptyList(), labels: List<String> = emptyList()) =
        Issue(
            id = id, title = "t $id", status = status, priority = p, type = type, parent = parent, labels = labels,
            dependencies = blocks.map { Dependency(id, it, "blocks") } + listOfNotNull(parent.takeIf { it.isNotEmpty() }?.let { Dependency(id, it, "parent-child") }),
        )

    @Test fun openBeadWaitingOnOpenBlockerIsBlocked() {
        val ix = BoardIndex(listOf(issue("A"), issue("B", blocks = listOf("A")), issue("C", blocks = listOf("D")), issue("D", status = "closed")))
        assertEquals(Lane.READY, ix.laneOf(ix.byId["A"]!!))
        assertEquals(Lane.BLOCKED, ix.laneOf(ix.byId["B"]!!))
        assertEquals(Lane.READY, ix.laneOf(ix.byId["C"]!!), "a closed blocker no longer blocks")
        assertEquals(Lane.DONE, ix.laneOf(ix.byId["D"]!!))
    }

    @Test fun parentChildEdgesAreNotBlockers() {
        val ix = BoardIndex(listOf(issue("E", type = "epic"), issue("E.1", parent = "E")))
        assertEquals(Lane.READY, ix.laneOf(ix.byId["E.1"]!!))
        assertEquals("E", ix.epicOf(ix.byId["E.1"]!!)?.id)
        assertEquals(1, ix.epics.single().total)
    }

    @Test fun needsYouSortsFirstThenPriority() {
        val ix = BoardIndex(listOf(issue("X", p = 0), issue("Y", p = 3, labels = listOf("human")), issue("Z", p = 1)))
        val ready = ix.columns(GroupBy.WORKFLOW, BoardFilter()).first { it.key == "ready" }
        assertEquals(listOf("Y", "X", "Z"), ready.issues.map { it.id })
    }

    @Test fun epicFilterKeepsDescendants() {
        val ix = BoardIndex(listOf(issue("E", type = "epic"), issue("E.1", parent = "E"), issue("E.1.1", parent = "E.1"), issue("L")))
        val ids = ix.issues.filter { ix.matches(it, BoardFilter(epic = "E")) }.map { it.id }.toSet()
        assertEquals(setOf("E", "E.1", "E.1.1"), ids)
        val loose = ix.issues.filter { ix.matches(it, BoardFilter(epic = BoardFilter.NO_EPIC)) }.map { it.id }
        assertEquals(listOf("L"), loose)
    }

    @Test fun naturalIdOrder() {
        val ids = listOf("NAC-10", "NAC-2", "NAC-1.10", "NAC-1.2").sortedWith { a, b -> BoardIndex.naturalCompare(a, b) }
        assertEquals(listOf("NAC-1.2", "NAC-1.10", "NAC-2", "NAC-10"), ids)
    }

    @Test fun moveActions() {
        assertEquals(MoveAction.Close, moveActionFor("done"))
        assertEquals(MoveAction.SetStatus("in_progress"), moveActionFor("doing"))
        assertEquals(MoveAction.SetPriority(0), moveActionFor("p0"))
        assertEquals(MoveAction.SetType("bug"), moveActionFor("t:bug"))
        assertTrue(moveActionFor("nonsense") == null)
    }
}
