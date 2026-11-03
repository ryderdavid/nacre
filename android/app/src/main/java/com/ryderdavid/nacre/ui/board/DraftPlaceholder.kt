package com.ryderdavid.nacre.ui.board

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.board.LaneItem

/**
 * A proposal that doesn't exist yet, in place of its seeds' cards. It shows no title, change name,
 * agent or computer, and has no click or drag handlers.
 */
@Composable
fun DraftPlaceholderCard(item: LaneItem.DraftPlaceholder, modifier: Modifier = Modifier) {
    val failed = item.draft.failed
    val colors = MaterialTheme.colorScheme
    OutlinedCard(
        shape = RoundedCornerShape(16.dp),
        colors = CardDefaults.outlinedCardColors(containerColor = if (failed) colors.errorContainer else colors.surfaceContainerLow),
        border = BorderStroke(1.dp, if (failed) colors.error else colors.outlineVariant.copy(alpha = 0.6f)),
        modifier = modifier.fillMaxWidth().semantics { contentDescription = if (failed) "Drafting failed" else "Drafting…" },
    ) {
        Column(Modifier.padding(horizontal = 12.dp, vertical = 14.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                if (!failed) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                Text(
                    if (failed) "Drafting failed" else "Drafting…",
                    style = MaterialTheme.typography.bodyLarge,
                    color = if (failed) colors.onErrorContainer else colors.onSurfaceVariant,
                )
            }
            if (failed && item.draft.message.isNotBlank()) {
                Text(
                    item.draft.message,
                    style = MaterialTheme.typography.bodySmall,
                    color = colors.onErrorContainer,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(top = 4.dp),
                )
            }
        }
    }
}
