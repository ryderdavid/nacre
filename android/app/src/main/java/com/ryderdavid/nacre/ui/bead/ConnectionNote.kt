package com.ryderdavid.nacre.ui.bead

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import com.ryderdavid.nacre.data.claude.BeadChatItem

/** A dropped connection in a bead's chat: when it went, and when it came back or that it didn't. */
@Composable
internal fun ConnectionNote(item: BeadChatItem.Connection) {
    Text(
        item.text,
        style = MaterialTheme.typography.labelMedium,
        color = if (item.failed) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.fillMaxWidth(),
    )
}
