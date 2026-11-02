package com.ryderdavid.nacre.ui.bead

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ThreadComposerTest {
    @Test fun askingForAnEditProposesChangesAndAQuestionIsAnswered() {
        assertTrue(wantsChanges("Make it agent-ready"))
        assertTrue(wantsChanges("  tighten the description"))
        assertFalse(wantsChanges("What's left to do?"))
        assertFalse(wantsChanges("Will sorting by start date break Ready?"))
    }

    @Test fun aSignedOutComputerIsNamed() {
        assertEquals("RCobean-GMWN73D2T5", com.ryderdavid.nacre.ui.common.signedOutOn("Claude is signed out on RCobean-GMWN73D2T5. Sign it in from the app with Sign in to Claude, then try again."))
        assertNull(com.ryderdavid.nacre.ui.common.signedOutOn("claude took too long and was stopped"))
    }
}
