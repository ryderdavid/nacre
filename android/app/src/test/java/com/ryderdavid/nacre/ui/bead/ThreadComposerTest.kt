package com.ryderdavid.nacre.ui.bead

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ThreadComposerTest {
    @Test fun askingForAnEditProposesChangesAndAQuestionIsAnswered() {
        assertTrue(wantsChanges("Make it agent-ready"))
        assertTrue(wantsChanges("  tighten the description"))
        assertFalse(wantsChanges("What's left to do?"))
        assertFalse(wantsChanges("Will sorting by start date break Ready?"))
    }
}
