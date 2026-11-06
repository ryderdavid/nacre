package com.ryderdavid.nacre.ui.theme

import android.content.Context
import android.provider.Settings
import androidx.compose.runtime.staticCompositionLocalOf

/** True when the phone asks for reduced motion (animations off): shape morphs and springs become crossfades. */
val LocalReducedMotion = staticCompositionLocalOf { false }

/** Whether the system's animator duration scale is zero ("Remove animations"). */
fun reducedMotion(context: Context): Boolean =
    runCatching { Settings.Global.getFloat(context.contentResolver, Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f }.getOrDefault(false)
