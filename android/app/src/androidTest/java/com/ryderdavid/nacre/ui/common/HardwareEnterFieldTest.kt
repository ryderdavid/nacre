package com.ryderdavid.nacre.ui.common

import android.os.SystemClock
import android.view.KeyCharacterMap
import android.view.KeyEvent as NativeKeyEvent
import android.view.InputDevice
import androidx.compose.material3.OutlinedTextField
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.key.KeyEvent
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performKeyPress
import androidx.compose.ui.test.performTextInput
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test

/**
 * Drives a real multi-line text field with [sendOnHardwareEnter] the way the message, comment and chat
 * bars use it, injecting key events as a hardware keyboard and as the on-screen keyboard would.
 */
class HardwareEnterFieldTest {
    @get:Rule val rule = createComposeRule()

    private var text by mutableStateOf("")
    private val sent = mutableStateListOf<String>()

    private fun setUp(initial: String) {
        text = initial
        rule.setContent {
            OutlinedTextField(
                value = text, onValueChange = { text = it }, maxLines = 5,
                modifier = Modifier.testTag("field").sendOnHardwareEnter(text.isNotBlank()) { sent += text.trim(); text = "" },
            )
        }
        rule.onNodeWithTag("field").performClick()
    }

    /** A down/up pair for Enter, from a hardware keyboard unless [soft]. */
    private fun enter(shift: Boolean = false, soft: Boolean = false) {
        val now = SystemClock.uptimeMillis()
        val meta = if (shift) NativeKeyEvent.META_SHIFT_ON or NativeKeyEvent.META_SHIFT_LEFT_ON else 0
        val device = if (soft) KeyCharacterMap.VIRTUAL_KEYBOARD else 1
        val flags = if (soft) NativeKeyEvent.FLAG_SOFT_KEYBOARD or NativeKeyEvent.FLAG_KEEP_TOUCH_MODE else 0
        for (action in listOf(NativeKeyEvent.ACTION_DOWN, NativeKeyEvent.ACTION_UP)) {
            val e = NativeKeyEvent(now, now, action, NativeKeyEvent.KEYCODE_ENTER, 0, meta, device, 0, flags, InputDevice.SOURCE_KEYBOARD)
            rule.onNodeWithTag("field").performKeyPress(KeyEvent(e))
        }
        rule.waitForIdle()
    }

    @Test fun hardwareEnterSendsAndClears() {
        setUp("")
        rule.onNodeWithTag("field").performTextInput("hello")
        enter()
        assertEquals(listOf("hello"), sent.toList())
        assertEquals("", text)
    }

    @Test fun hardwareShiftEnterAddsANewlineAndDoesNotSend() {
        setUp("")
        rule.onNodeWithTag("field").performTextInput("ab")
        enter(shift = true)
        rule.onNodeWithTag("field").performTextInput("cd")
        assertEquals(emptyList<String>(), sent.toList())
        assertEquals("ab\ncd", text)
    }

    @Test fun hardwareEnterOnBlankTextDoesNothing() {
        setUp("")
        rule.onNodeWithTag("field").performTextInput("   ")
        enter()
        assertEquals(emptyList<String>(), sent.toList())
        assertEquals("   ", text)
    }

    @Test fun hardwareEnterOnEmptyFieldDoesNothing() {
        setUp("")
        enter()
        assertEquals(emptyList<String>(), sent.toList())
        assertEquals("", text)
    }

    @Test fun onScreenEnterAddsANewlineAndDoesNotSend() {
        setUp("")
        rule.onNodeWithTag("field").performTextInput("ab")
        enter(soft = true)
        rule.onNodeWithTag("field").performTextInput("cd")
        assertEquals(emptyList<String>(), sent.toList())
        assertEquals("ab\ncd", text)
    }
}
