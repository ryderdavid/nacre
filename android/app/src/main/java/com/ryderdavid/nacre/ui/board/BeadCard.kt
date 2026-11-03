package com.ryderdavid.nacre.ui.board

import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Comment
import androidx.compose.material.icons.rounded.Devices
import androidx.compose.material.icons.rounded.HourglassTop
import androidx.compose.material.icons.rounded.Terminal
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.model.ActiveElsewhere
import com.ryderdavid.nacre.data.model.ChangesList
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.elsewhere
import com.ryderdavid.nacre.ui.common.BeadId
import com.ryderdavid.nacre.ui.common.LabelChip
import com.ryderdavid.nacre.ui.common.NeedsYouTag
import com.ryderdavid.nacre.ui.common.SignoffTag
import com.ryderdavid.nacre.ui.common.PriorityPill
import com.ryderdavid.nacre.ui.common.TypeLabel
import com.ryderdavid.nacre.ui.common.relativeTime
import com.ryderdavid.nacre.ui.theme.Nacre

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun BeadCard(
    issue: Issue,
    index: BoardIndex,
    running: Boolean,
    selected: Boolean,
    modifier: Modifier = Modifier,
    lifted: Boolean = false,
    /** The board's changes, for the spec chip; null on older bridges. */
    changes: ChangesList? = null,
) {
    val toCheck = remember(index) { index.issues.filter { it.awaitsSignoff }.mapTo(HashSet()) { it.id } }
    val chip = specChip(issue, changes, toCheck)
    val blockers = if (!issue.isClosed) index.openBlockers(issue) else emptyList()
    val crumb = index.breadcrumb(issue)
    val elsewhere = issue.elsewhere(liveRun = running)
    val accent = when {
        running -> Nacre.colors.running
        issue.needsYou -> Nacre.colors.needsYou
        else -> null
    }
    Surface(
        shape = RoundedCornerShape(16.dp),
        color = if (selected) MaterialTheme.colorScheme.secondaryContainer else MaterialTheme.colorScheme.surfaceContainerLowest,
        border = BorderStroke(1.dp, if (selected) MaterialTheme.colorScheme.secondary else MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.6f)),
        shadowElevation = if (lifted) 12.dp else 0.dp,
        tonalElevation = if (lifted) 3.dp else 0.dp,
        modifier = modifier.fillMaxWidth().semantics { contentDescription = "${issue.id}: ${issue.title}, priority ${issue.priority}, ${issue.type}" },
    ) {
        Row(Modifier.height(IntrinsicSize.Min)) {
            if (accent != null) Box(Modifier.width(4.dp).fillMaxHeight().background(accent))
            Column(Modifier.padding(start = 12.dp, end = 12.dp, top = 10.dp, bottom = 10.dp).weight(1f)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    PriorityPill(issue.priority)
                    BeadId(issue.id, Modifier.weight(1f))
                    TypeLabel(issue.type)
                    if (running) RunningBadge()
                }
                if (issue.needsYou) {
                    Spacer(Modifier.height(6.dp))
                    if (issue.awaitsSignoff) SignoffTag() else NeedsYouTag()
                }
                if (elsewhere != null) {
                    Spacer(Modifier.height(6.dp))
                    ElsewhereTag(elsewhere)
                }
                Spacer(Modifier.height(6.dp))
                Text(
                    issue.title,
                    style = MaterialTheme.typography.bodyLarge.copy(fontWeight = FontWeight.Medium),
                    color = if (issue.isClosed) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface,
                    maxLines = 4,
                    overflow = TextOverflow.Ellipsis,
                )
                if (crumb.isNotEmpty()) {
                    Spacer(Modifier.height(4.dp))
                    Text(crumb, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
                if (blockers.isNotEmpty()) {
                    Spacer(Modifier.height(8.dp))
                    WaitsOn(blockers, index)
                }
                if (chip != null && chip.strip.isNotEmpty()) {
                    Spacer(Modifier.height(8.dp))
                    SectionStrip(chip.strip, toCheck = toCheck)
                } else if (issue.isEpic) {
                    val kids = index.descendants(issue.id)
                    if (kids.isNotEmpty()) {
                        val done = kids.count { it.isClosed }
                        Spacer(Modifier.height(8.dp))
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            LinearProgressIndicator(
                                progress = { done.toFloat() / kids.size },
                                strokeCap = StrokeCap.Round,
                                modifier = Modifier.weight(1f).height(6.dp),
                            )
                            Spacer(Modifier.width(8.dp))
                            Text("$done/${kids.size}", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                }
                val labels = issue.labels.filter { it != "human" && !isInternalCardLabel(it) && (chip == null || !isSpecLabel(it)) }
                val hasFooter = chip != null || labels.isNotEmpty() || issue.commentCount > 0 || issue.assignee.isNotBlank()
                if (hasFooter) {
                    Spacer(Modifier.height(8.dp))
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(4.dp), verticalArrangement = Arrangement.spacedBy(4.dp), itemVerticalAlignment = Alignment.CenterVertically) {
                        // The spec chip takes the first label slot, then two other labels.
                        val room = if (chip != null) 2 else 3
                        chip?.let { SpecChipView(it) }
                        labels.take(room).forEach { LabelChip(it) }
                        if (labels.size > room) LabelChip("+${labels.size - room}")
                        if (issue.commentCount > 0) {
                            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(start = 4.dp)) {
                                Icon(Icons.AutoMirrored.Rounded.Comment, null, Modifier.size(12.dp), tint = MaterialTheme.colorScheme.outline)
                                Text(" ${issue.commentCount}", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline)
                            }
                        }
                        if (issue.assignee.isNotBlank()) {
                            Text("· ${issue.assignee}", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline, maxLines = 1)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun WaitsOn(blockers: List<String>, index: BoardIndex) {
    Surface(color = MaterialTheme.colorScheme.surfaceContainerHigh, shape = RoundedCornerShape(10.dp), modifier = Modifier.fillMaxWidth()) {
        Row(Modifier.padding(horizontal = 8.dp, vertical = 6.dp)) {
            Icon(Icons.Rounded.HourglassTop, null, Modifier.size(14.dp).padding(top = 1.dp), tint = Nacre.colors.blocked)
            Spacer(Modifier.width(6.dp))
            Text(
                "Waits on " + blockers.take(3).joinToString("; ") { id -> index.byId[id]?.let { "$id ${it.title.take(48)}" } ?: id } +
                    if (blockers.size > 3) " +${blockers.size - 3} more" else "",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 3,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

/** bd shows the bead claimed while no run here works on it: by whom (unless the user), and since when. */
@Composable
private fun ElsewhereTag(e: ActiveElsewhere) {
    val text = listOfNotNull(e.title, relativeTime(e.since).ifBlank { null }).joinToString(" · ")
    Row(verticalAlignment = Alignment.CenterVertically) {
        Icon(Icons.Rounded.Devices, null, Modifier.size(14.dp), tint = Nacre.colors.doing)
        Spacer(Modifier.width(4.dp))
        Text(text, style = MaterialTheme.typography.labelSmall, color = Nacre.colors.doing, maxLines = 1, overflow = TextOverflow.Ellipsis)
    }
}

@Composable
private fun RunningBadge() {
    val t = rememberInfiniteTransition(label = "run")
    val a by t.animateFloat(0.45f, 1f, infiniteRepeatable(tween(900), RepeatMode.Reverse), label = "pulse")
    Surface(color = Nacre.colors.running.copy(alpha = 0.16f), contentColor = Nacre.colors.running, shape = RoundedCornerShape(8.dp)) {
        Row(Modifier.padding(horizontal = 6.dp, vertical = 2.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Rounded.Terminal, "Running this bead", Modifier.size(12.dp).alpha(a))
            Text(" Running", style = MaterialTheme.typography.labelSmall)
        }
    }
}

/** Internal orchestration labels never belong on a board card. */
fun isInternalCardLabel(label: String): Boolean =
    listOf("specflow:", "round:", "spec-section:", "nacre-").any(label::startsWith)
