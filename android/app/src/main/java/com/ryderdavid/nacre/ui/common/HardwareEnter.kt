package com.ryderdavid.nacre.ui.common

import android.view.KeyCharacterMap
import android.view.KeyEvent as NativeKeyEvent
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.isShiftPressed
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type

/** What a key press in a message field should do. */
internal enum class EnterAction { PassThrough, Send, Swallow }

/**
 * Hardware Enter sends when it can and is swallowed when it can't, so it never breaks the line;
 * Shift+Enter breaks the line. Anything else, the on-screen keyboard's Enter included, is left alone.
 */
internal fun enterAction(isEnter: Boolean, isHardware: Boolean, shift: Boolean, keyDown: Boolean, canSend: Boolean): EnterAction = when {
    !isEnter || !isHardware || shift -> EnterAction.PassThrough
    keyDown && canSend -> EnterAction.Send
    else -> EnterAction.Swallow
}

/**
 * Enter-to-send for the message, comment and chat bar fields. [canSend] should match the field's send
 * button, blank text included, and [onSend] should do what tapping it does.
 */
fun Modifier.sendOnHardwareEnter(canSend: Boolean, onSend: () -> Unit): Modifier = onPreviewKeyEvent { e ->
    val native = e.nativeKeyEvent
    // The on-screen keyboard's keys come from the virtual keyboard device, or are flagged as soft.
    val hardware = native.deviceId != KeyCharacterMap.VIRTUAL_KEYBOARD && native.flags and NativeKeyEvent.FLAG_SOFT_KEYBOARD == 0
    val action = enterAction(
        isEnter = e.key == Key.Enter || e.key == Key.NumPadEnter,
        isHardware = hardware,
        shift = e.isShiftPressed,
        keyDown = e.type == KeyEventType.KeyDown,
        canSend = canSend,
    )
    if (action == EnterAction.Send) onSend()
    action != EnterAction.PassThrough
}
