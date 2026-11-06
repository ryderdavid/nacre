package com.ryderdavid.nacre.ui.workspace.beads

import com.ryderdavid.nacre.data.board.BoardFilter
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.ui.workspace.WorkspaceRules
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** 9.1: the board by width, and filters that say what they hide. */
class BeadsLayoutTest {
    @Test fun columnsNeedRoomForTwoLanesOf272() {
        assertEquals(2 * 272 + 12 + 2 * 16, BeadsLayout.COLUMNS_MIN_WIDTH)
        assertFalse(BeadsLayout.usesColumns(412), "the cover screen shows one lane at a time")
        assertFalse(BeadsLayout.usesColumns(BeadsLayout.COLUMNS_MIN_WIDTH - 1))
        assertTrue(BeadsLayout.usesColumns(BeadsLayout.COLUMNS_MIN_WIDTH))
    }

    @Test fun focusedOnTheUnfoldedScreenShowsColumns() {
        // Focused, Beads gets the window less the rail and the pane's end padding.
        val focused = 840 - WorkspaceRules.RAIL - 8
        assertTrue(BeadsLayout.usesColumns(focused))
        // Beside chat at the Even preset, the pane is too narrow for two columns: the compact board.
        val usable = WorkspaceRules.usable(840)
        assertFalse(BeadsLayout.usesColumns(usable - WorkspaceRules.reset(usable)))
    }

    @Test fun columnsAreNeverNarrowerThan272() {
        for (w in BeadsLayout.COLUMNS_MIN_WIDTH..1600 step 7) for (n in 1..7) {
            val c = BeadsLayout.columnWidth(w, n)
            assertTrue(c >= BeadsLayout.COLUMN_MIN, "width $w with $n lanes gave $c")
            assertTrue(c <= BeadsLayout.COLUMN_MAX)
        }
        assertEquals(2, BeadsLayout.columnsThatFit(BeadsLayout.COLUMNS_MIN_WIDTH))
        // Columns that fit share the pane: 736dp holds two columns of 346dp.
        assertEquals((736 - 32 - 12) / 2, BeadsLayout.columnWidth(736, 7))
    }

    @Test fun theHeaderSaysWhichLanesAreInView() {
        assertEquals("Lanes 1–3 of 7", BeadsLayout.rangeLabel(0, 3, 7))
        assertEquals("Lanes 5–7 of 7", BeadsLayout.rangeLabel(4, 3, 7))
        assertEquals("", BeadsLayout.rangeLabel(0, 7, 7), "nothing to say when every lane shows")
    }

    private fun issue(id: String, type: String = "task", labels: List<String> = emptyList(), parent: String = "") =
        Issue(id = id, title = "t $id", type = type, labels = labels, parent = parent)

    @Test fun filtersSayWhatTheyHideAndClear() {
        val ix = BoardIndex(listOf(issue("E", type = "epic"), issue("E.1", parent = "E"), issue("B", type = "bug"), issue("H", labels = listOf("human")), issue("T")))
        assertNull(BeadsFilters.note(ix, BoardFilter()), "no note without a filter")
        assertEquals("4 beads hidden by Needs you", BeadsFilters.note(ix, BoardFilter(needsYou = true)))
        assertEquals("5 beads hidden by t E, Bugs", BeadsFilters.note(ix, BoardFilter(epic = "E", types = setOf("bug"))), "the epic has no bugs")
        assertEquals("3 beads hidden by t E", BeadsFilters.note(ix, BoardFilter(epic = "E")), "an epic keeps itself and its beads")
        assertEquals("1 bead hidden by not in an epic", BeadsFilters.note(BoardIndex(listOf(issue("E", type = "epic"), issue("L"))), BoardFilter(epic = BoardFilter.NO_EPIC)))
        assertEquals("4 beads hidden by “E.1”", BeadsFilters.note(ix, BoardFilter(query = " E.1 ")))
        assertEquals(listOf("t E", "Needs you", "Bugs", "Features", "“x”"), BeadsFilters.parts(BoardFilter(epic = "E", needsYou = true, types = setOf("feature", "bug"), query = "x")) { ix.byId[it]?.title })
    }
}
