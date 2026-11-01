package com.ryderdavid.nacre.ui.common

import kotlin.test.Test
import kotlin.test.assertEquals

class HardwareEnterTest {
    private fun act(isEnter: Boolean = true, isHardware: Boolean = true, shift: Boolean = false, keyDown: Boolean = true, canSend: Boolean = true) =
        enterAction(isEnter, isHardware, shift, keyDown, canSend)

    @Test fun hardwareEnterSends() = assertEquals(EnterAction.Send, act())

    @Test fun keyUpIsSwallowedWithoutSendingAgain() = assertEquals(EnterAction.Swallow, act(keyDown = false))

    @Test fun blankTextSwallowsEnterSoNoNewlineIsAdded() = assertEquals(EnterAction.Swallow, act(canSend = false))

    @Test fun shiftEnterBreaksTheLine() {
        assertEquals(EnterAction.PassThrough, act(shift = true))
        assertEquals(EnterAction.PassThrough, act(shift = true, canSend = false))
    }

    @Test fun onScreenEnterBreaksTheLine() {
        assertEquals(EnterAction.PassThrough, act(isHardware = false))
        assertEquals(EnterAction.PassThrough, act(isHardware = false, canSend = false))
    }

    @Test fun otherKeysAreLeftAlone() = assertEquals(EnterAction.PassThrough, act(isEnter = false))
}
