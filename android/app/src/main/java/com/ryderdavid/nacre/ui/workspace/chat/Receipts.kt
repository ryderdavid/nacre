package com.ryderdavid.nacre.ui.workspace.chat

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.ErrorOutline
import androidx.compose.material.icons.rounded.HourglassTop
import androidx.compose.material.icons.rounded.Schedule
import androidx.compose.material.icons.rounded.Terminal
import androidx.compose.material3.AssistChip
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.chat.CardState
import com.ryderdavid.nacre.data.chat.ChatItem
import com.ryderdavid.nacre.data.chat.PermissionCard
import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.model.DecisionAnswer
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.workspace.decisions.DecisionModel
import kotlinx.coroutines.launch

/** What a decision's one-line receipt says. */
object Receipts {
    fun line(d: Decision): String = when (d.state) {
        "pending" -> "Waiting for you: ${d.title}"
        "deferred" -> "Skipped: ${d.title}"
        else -> d.title + " · " + (DecisionModel.stateLine(d) ?: d.state)
    }

    fun permissionLine(card: PermissionCard): String = when (card.state) {
        CardState.PENDING -> "Waiting for you: ${card.tool} · ${card.step}"
        else -> "${card.outcomeLine}: ${card.tool} · ${card.step}"
    }
}

private fun receiptIcon(state: String): ImageVector = when (state) {
    "done" -> Icons.Rounded.CheckCircle
    "executing" -> Icons.Rounded.Schedule
    "pending", "held", "deferred" -> Icons.Rounded.HourglassTop
    else -> Icons.Rounded.ErrorOutline
}

/**
 * A decision as the conversation records it: one line where it was raised or asked, kept up to date
 * with nacred's state, that opens to what it does and nacred's message. Its options are above the
 * composer while it waits, never here.
 */
@OptIn(ExperimentalLayoutApi::class, ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun DecisionReceipt(d: Decision, modifier: Modifier = Modifier) {
    val container = LocalContainer.current
    var open by rememberSaveable(d.key) { mutableStateOf(false) }
    val c = MaterialTheme.colorScheme
    val bad = d.state in setOf("failed", "unknown", "expired", "declined")
    Surface(
        onClick = { open = !open }, shape = RoundedCornerShape(16.dp), color = c.surfaceContainerLow,
        modifier = modifier.fillMaxWidth().heightIn(min = 48.dp).semantics {
            liveRegion = LiveRegionMode.Polite
            contentDescription = Receipts.line(d) + if (open) ". Hide details" else ". Show details"
        },
    ) {
        Column(Modifier.padding(horizontal = 14.dp, vertical = 10.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Icon(receiptIcon(d.state), null, Modifier.size(18.dp), tint = if (bad) c.error else if (d.state == "done") c.primary else c.onSurfaceVariant)
                Text(Receipts.line(d), style = MaterialTheme.typography.bodyMedium, color = c.onSurfaceVariant, maxLines = if (open) 4 else 1, overflow = TextOverflow.Ellipsis)
            }
            if (open) {
                if (d.subtitle.isNotBlank()) Text(d.subtitle, style = MaterialTheme.typography.bodySmall, color = c.onSurfaceVariant)
                d.consequences.forEach { Text("· $it", style = MaterialTheme.typography.bodySmall) }
                (d.result?.message?.ifBlank { null } ?: d.reason.ifBlank { null })?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (DecisionModel.canRenew(d)) AssistChip(onClick = {
                        container.scope.launch { runCatching { container.repository.renewDecision(d) }.onFailure { container.repository.notify("Couldn't ask again: ${it.message}") } }
                    }, label = { Text("Ask again") }, modifier = Modifier.heightIn(min = 48.dp))
                    if (DecisionModel.canAcknowledge(d)) AssistChip(onClick = {
                        container.scope.launch { runCatching { container.repository.answerDecision(d, DecisionAnswer(acknowledge = true)) } }
                    }, label = { Text("Got it") }, modifier = Modifier.heightIn(min = 48.dp))
                }
            }
        }
    }
}

/** A permission request as the conversation records it; its Allow and Deny are above the composer. */
@Composable
fun PermissionReceipt(card: PermissionCard, modifier: Modifier = Modifier) {
    val c = MaterialTheme.colorScheme
    Row(modifier.fillMaxWidth().padding(horizontal = 4.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
        Icon(Icons.Rounded.Terminal, null, Modifier.size(16.dp), tint = if (card.pending) c.tertiary else c.onSurfaceVariant)
        Text(Receipts.permissionLine(card), style = MaterialTheme.typography.bodySmall, color = c.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
    }
}

/**
 * A new chat's heads-up from nacred: what waits, and whether to go through it now. Yes sends /needs-you;
 * Not now folds it to one line that can still start it. Once the conversation has started it only reads.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun HeadsUp(item: ChatItem.HeadsUp, answerable: Boolean, onYes: () -> Unit) {
    var later by rememberSaveable(item.key) { mutableStateOf(false) }
    val c = MaterialTheme.colorScheme
    if (later || !answerable) {
        Surface(
            onClick = onYes, enabled = answerable, shape = RoundedCornerShape(16.dp), color = c.surfaceContainerLow,
            modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp),
        ) {
            Row(Modifier.padding(horizontal = 14.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(
                    "${item.total} need${if (item.total == 1) "s" else ""} you" + if (answerable) " · Go through them" else "",
                    style = MaterialTheme.typography.bodyMedium, color = c.onSurfaceVariant,
                )
            }
        }
        return
    }
    Surface(shape = RoundedCornerShape(24.dp), color = c.surfaceContainerHigh, modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(item.text, style = MaterialTheme.typography.bodyLarge)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(selected = true, onClick = onYes, label = { Text("Yes, one by one") }, modifier = Modifier.heightIn(min = 48.dp))
                AssistChip(onClick = { later = true }, label = { Text("Not now") }, modifier = Modifier.heightIn(min = 48.dp))
            }
        }
    }
}
