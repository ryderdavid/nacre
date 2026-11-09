package com.ryderdavid.nacre.ui.workspace.beads

import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.model.Issue
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** A child bead's card names its parent, so "12. Device checks (human)" says what it checks. */
class ParentLineTest {
    private val epic = Issue(id = "NAC-4qu", title = "Chat first: reorient Nacre to Chat · Spec · Beads")
    private val child = Issue(id = "NAC-4qu.12", title = "12. Device checks (human)", parent = "NAC-4qu")

    @Test fun aChildNamesItsParentByTitle() {
        assertEquals("In Chat first: reorient Nacre to Chat · Spec · Beads", parentLine(child, BoardIndex(listOf(epic, child))))
    }

    @Test fun aParentOffTheBoardIsNamedById() {
        assertEquals("In NAC-4qu", parentLine(child, BoardIndex(listOf(child))))
    }

    @Test fun aBeadWithoutAParentHasNoLine() {
        assertNull(parentLine(epic, BoardIndex(listOf(epic, child))))
    }
}
