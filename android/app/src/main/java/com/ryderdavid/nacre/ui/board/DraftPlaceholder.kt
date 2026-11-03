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
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.TextButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.width
import com.ryderdavid.nacre.data.board.LaneItem

/** What a failed draft placeholder's buttons do; [busy] holds change IDs with a request on its way. */
class DraftHandlers(
    val busy: Set<String> = emptySet(),
    val onResume: (LaneItem.DraftPlaceholder) -> Unit = {},
    val onDismiss: (LaneItem.DraftPlaceholder) -> Unit = {},
)

/**
 * A proposal that doesn't exist yet, in place of its seeds' cards. It shows no title, change name,
 * agent or computer, and has no click or drag handlers. A failed draft is drawn in the error colour
 * with "Drafting failed", nacred's one-line message, the change's Resume when offered, and Dismiss.
 */
@Composable
fun DraftPlaceholderCard(item: LaneItem.DraftPlaceholder, handlers: DraftHandlers = DraftHandlers(), modifier: Modifier = Modifier) {
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
            if (failed) {
                val busy = item.changeId in handlers.busy
                Row(Modifier.fillMaxWidth().padding(top = 8.dp), horizontalArrangement = Arrangement.End, verticalAlignment = Alignment.CenterVertically) {
                    TextButton(
                        onClick = { handlers.onDismiss(item) }, enabled = !busy,
                        colors = ButtonDefaults.textButtonColors(contentColor = colors.onErrorContainer),
                    ) { Text("Dismiss") }
                    if (item.resumable) {
                        Spacer(Modifier.width(8.dp))
                        FilledTonalButton(
                            onClick = { handlers.onResume(item) }, enabled = !busy,
                            colors = ButtonDefaults.filledTonalButtonColors(containerColor = colors.error, contentColor = colors.onError),
                        ) { Text("Resume") }
                    }
                }
            }
        }
    }
}
