package com.ryderdavid.nacre.data.board

import com.ryderdavid.nacre.data.claude.DraftTurn
import com.ryderdavid.nacre.data.model.FoundComment
import com.ryderdavid.nacre.data.model.NewEffortPrefill
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

    @Test fun anUntouchedPrefillIsNotKeptButAnEditedOneIs() {
        val drafts = NewBeadDrafts()
        val start = NewBeadForm(title = "Found: flaky test", description = "seen in NAC-9")
        drafts.put("k", start, start)
        assertNull(drafts.get("k"), "nothing was typed yet")
        val edited = start.copy(description = "seen in NAC-9; only on CI")
        drafts.put("k", edited, start)
        assertEquals(edited, drafts.get("k"))
    }

    @Test fun eachKindOfFormKeepsItsOwnDraft() {
        val keys = listOf(
            newBeadDraftKey("mac~r1", effort = false, prefill = null),
            newBeadDraftKey("mac~r1", effort = true, prefill = null),
            newBeadDraftKey("mac~r1", effort = true, prefill = NewEffortPrefill(title = "x", followOnOf = "change-a")),
            newBeadDraftKey("mac~r1", effort = true, prefill = NewEffortPrefill(title = "x", foundComment = FoundComment("NAC-1", "c1"))),
            newBeadDraftKey("mac~r1", effort = true, prefill = NewEffortPrefill(title = "x", foundComment = FoundComment("NAC-1", "c2"))),
            newBeadDraftKey("mac~r2", effort = false, prefill = null),
        )
        assertEquals(keys.size, keys.toSet().size)
        assertEquals("mac~r1", keys[0], "a plain new bead keeps the key it had before")
    }
}
