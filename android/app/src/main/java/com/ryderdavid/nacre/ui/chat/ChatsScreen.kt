package com.ryderdavid.nacre.ui.chat

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.LargeTopAppBar
import androidx.compose.material3.ListItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier

/** "New · <label>": the Chat list's first row, about the screen shown before Chat. */
fun newChatLabel(originLabel: String) = "New · $originLabel"

/**
 * The Chat tab. [originLabel] names the screen last shown before switching here (or all boards).
 * The conversation and the list of past chats arrive with the chat endpoints.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ChatsScreen(originLabel: String) {
    Scaffold(topBar = { LargeTopAppBar(title = { Text("Chat") }) }) { pad ->
        LazyColumn(Modifier.padding(pad).fillMaxSize()) {
            item {
                ListItem(
                    headlineContent = { Text(newChatLabel(originLabel)) },
                    supportingContent = { Text("Starting a chat from here isn't available yet.") },
                    leadingContent = { Icon(Icons.Rounded.Add, null) },
                )
            }
        }
    }
}
