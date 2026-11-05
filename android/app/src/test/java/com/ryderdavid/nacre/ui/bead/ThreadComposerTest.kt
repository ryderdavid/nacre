package com.ryderdavid.nacre.ui.bead

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ThreadComposerTest {

    @Test fun aSignedOutComputerIsNamed() {
        assertEquals("RCobean-GMWN73D2T5", com.ryderdavid.nacre.ui.common.signedOutOn("Claude is signed out on RCobean-GMWN73D2T5. Sign it in from the app with Sign in to Claude, then try again."))
        assertNull(com.ryderdavid.nacre.ui.common.signedOutOn("claude took too long and was stopped"))
    }
}
