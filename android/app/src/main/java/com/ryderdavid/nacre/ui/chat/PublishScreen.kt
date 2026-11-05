package com.ryderdavid.nacre.ui.chat

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import com.ryderdavid.nacre.data.chat.ShownScreen
import com.ryderdavid.nacre.ui.ScreenOrigin
import com.ryderdavid.nacre.ui.common.LocalContainer

/** Publishes what this screen shows into the screen-context holder while it is shown, and again as it changes. */
@Composable
fun PublishScreen(origin: ScreenOrigin, screen: ShownScreen) {
    val holder = LocalContainer.current.chats.screens
    LaunchedEffect(origin, screen) { holder.publish(origin, screen) }
}
