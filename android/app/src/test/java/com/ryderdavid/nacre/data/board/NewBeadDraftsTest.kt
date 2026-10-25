package com.ryderdavid.nacre.data.board

import com.ryderdavid.nacre.data.claude.DraftTurn
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** A dismissed new-bead form keeps its input for the session (NAC-cjm). */
class NewBeadDraftsTest {
    @Test fun aDismissedFormComesBackAsItWas() {
        val drafts = NewBeadDrafts()
        val form = NewBeadForm(title = "Fix login", description = "rough note", priority = 1, parent = "NAC-1")
        drafts.put("mac~r1", form)
        assertEquals(form, drafts.get("mac~r1"))
    }

    @Test fun eachBoardKeepsItsOwnDraft() {
        val drafts = NewBeadDrafts()
        drafts.put("mac~r1", NewBeadForm(title = "one"))
        drafts.put("mac~r2", NewBeadForm(title = "two"))
        assertEquals("one", drafts.get("mac~r1")?.title)
        assertEquals("two", drafts.get("mac~r2")?.title)
    }

    @Test fun anEmptyFormLeavesNothingBehind() {
        val drafts = NewBeadDrafts()
        drafts.put("mac~r1", NewBeadForm(title = "one"))
        drafts.put("mac~r1", NewBeadForm(title = "  ", type = "bug", priority = 0))
        assertNull(drafts.get("mac~r1"), "chips alone aren't typed input")
    }

    @Test fun aClaudeDraftOrHalfWrittenMessageCounts() {
        assert(NewBeadForm(drafted = true).hasContent)
        assert(NewBeadForm(message = "make it P1").hasContent)
        assert(NewBeadForm(chat = listOf(DraftTurn(true, "hi"))).hasContent)
    }

    @Test fun clearingForgetsTheDraft() {
        val drafts = NewBeadDrafts()
        drafts.put("mac~r1", NewBeadForm(title = "one"))
        drafts.clear("mac~r1")
        assertNull(drafts.get("mac~r1"))
    }
}
