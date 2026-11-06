package com.ryderdavid.nacre.ui.common

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Chat
import androidx.compose.material.icons.rounded.Code
import androidx.compose.material.icons.rounded.KeyboardArrowDown
import androidx.compose.material.icons.rounded.PushPin
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.Icon
import androidx.compose.material3.SplitButtonDefaults
import androidx.compose.material3.SplitButtonLayout
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.chat.DefaultAgent
import com.ryderdavid.nacre.data.model.ChatWire

/**
 * New chat as a split button: the leading half starts a chat with the default agent in one tap, the arrow
 * opens the other agent and a way to make it the default.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun NewChatButton(onStart: (agent: String) -> Unit, modifier: Modifier = Modifier, height: Dp = 56.dp) {
    val context = LocalContext.current
    val default by DefaultAgent.state(context)
    val other = DefaultAgent.other(default)
    var menu by remember { mutableStateOf(false) }
    Box(modifier) {
        SplitButtonLayout(
            leadingButton = {
                SplitButtonDefaults.LeadingButton(
                    onClick = { onStart(default) },
                    modifier = Modifier.heightIn(min = height).semantics { contentDescription = "New ${DefaultAgent.label(default)} chat" },
                ) {
                    Icon(agentIcon(default), null, Modifier.size(SplitButtonDefaults.LeadingIconSize))
                    Text(DefaultAgent.label(default))
                }
            },
            trailingButton = {
                SplitButtonDefaults.TrailingButton(
                    checked = menu, onCheckedChange = { menu = it },
                    modifier = Modifier.heightIn(min = height).semantics { contentDescription = "More ways to start a chat" },
                ) { Icon(Icons.Rounded.KeyboardArrowDown, null) }
            },
        )
        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
            DropdownMenuItem(
                text = { Text(DefaultAgent.label(other)) }, leadingIcon = { Icon(agentIcon(other), null) },
                onClick = { menu = false; onStart(other) },
            )
            DropdownMenuItem(
                text = { Text("Make ${DefaultAgent.label(other)} default") }, leadingIcon = { Icon(Icons.Rounded.PushPin, null) },
                onClick = { menu = false; DefaultAgent.set(context, other) },
            )
        }
    }
}

fun agentIcon(agent: String) = if (agent == ChatWire.AGENT_CODEX) Icons.Rounded.Code else Icons.AutoMirrored.Rounded.Chat
