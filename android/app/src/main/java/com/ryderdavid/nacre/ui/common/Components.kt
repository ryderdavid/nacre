package com.ryderdavid.nacre.ui.common

import android.text.format.DateUtils
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Assignment
import androidx.compose.material.icons.rounded.AccountTree
import androidx.compose.material.icons.rounded.AutoAwesome
import androidx.compose.material.icons.rounded.BugReport
import androidx.compose.material.icons.rounded.Build
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.CloudOff
import androidx.compose.material.icons.rounded.Flag
import androidx.compose.material.icons.rounded.Gavel
import androidx.compose.material.icons.rounded.Science
import androidx.compose.material.icons.rounded.Star
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.Icon
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.mikepenz.markdown.m3.Markdown
import com.ryderdavid.nacre.ui.theme.MonoFamily
import com.ryderdavid.nacre.ui.theme.Nacre
import java.time.Instant

@Composable
fun BeadId(id: String, modifier: Modifier = Modifier, color: Color = MaterialTheme.colorScheme.onSurfaceVariant) {
    Text(
        id,
        modifier = modifier,
        style = MaterialTheme.typography.labelMedium.copy(fontFamily = MonoFamily, letterSpacing = 0.sp),
        color = color,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
    )
}

@Composable
fun PriorityPill(priority: Int, modifier: Modifier = Modifier) {
    val c = Nacre.colors.priority[priority.coerceIn(0, 4)]
    Surface(color = c, contentColor = MaterialTheme.colorScheme.surface, shape = RoundedCornerShape(6.dp), modifier = modifier) {
        Text(
            "P$priority",
            style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Bold),
            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
        )
    }
}

fun typeIcon(type: String): ImageVector = when (type) {
    "bug" -> Icons.Rounded.BugReport
    "feature", "story" -> Icons.Rounded.Star
    "epic" -> Icons.Rounded.AccountTree
    "decision" -> Icons.Rounded.Gavel
    "chore" -> Icons.Rounded.Build
    "spike" -> Icons.Rounded.Science
    "milestone" -> Icons.Rounded.Flag
    else -> Icons.AutoMirrored.Rounded.Assignment
}

@Composable
fun TypeLabel(type: String, modifier: Modifier = Modifier) {
    val c = Nacre.colors.type(type)
    Row(verticalAlignment = Alignment.CenterVertically, modifier = modifier) {
        Icon(typeIcon(type), contentDescription = null, tint = c, modifier = Modifier.size(14.dp))
        Spacer(Modifier.size(3.dp))
        Text(type.uppercase(), style = MaterialTheme.typography.labelSmall.copy(letterSpacing = 0.6.sp), color = c, maxLines = 1)
    }
}

@Composable
fun NeedsYouTag(modifier: Modifier = Modifier) {
    Surface(color = Nacre.colors.needsYouContainer, contentColor = Nacre.colors.needsYou, shape = RoundedCornerShape(6.dp), modifier = modifier) {
        Text("Needs you", style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.SemiBold), modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp))
    }
}

@Composable
fun Dot(color: Color, size: Int = 8) {
    Box(Modifier.size(size.dp).clip(CircleShape).background(color))
}

@Composable
fun LabelChip(text: String, modifier: Modifier = Modifier) {
    Surface(
        color = MaterialTheme.colorScheme.surfaceContainerHighest,
        contentColor = MaterialTheme.colorScheme.onSurfaceVariant,
        shape = RoundedCornerShape(6.dp),
        modifier = modifier,
    ) {
        Text(text, style = MaterialTheme.typography.labelSmall, maxLines = 1, modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp))
    }
}

@Composable
fun MarkdownText(text: String, modifier: Modifier = Modifier) {
    Markdown(content = text, modifier = modifier)
}

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun LoadingState(label: String? = null, modifier: Modifier = Modifier) {
    Column(modifier.fillMaxSize(), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.Center) {
        LoadingIndicator()
        if (label != null) {
            Spacer(Modifier.height(12.dp))
            Text(label, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

@Composable
fun MessageState(
    icon: ImageVector,
    title: String,
    body: String? = null,
    action: String? = null,
    onAction: (() -> Unit)? = null,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier.fillMaxSize().padding(32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        Surface(shape = CircleShape, color = MaterialTheme.colorScheme.secondaryContainer, modifier = Modifier.size(72.dp)) {
            Box(contentAlignment = Alignment.Center) {
                Icon(icon, null, tint = MaterialTheme.colorScheme.onSecondaryContainer, modifier = Modifier.size(32.dp))
            }
        }
        Spacer(Modifier.height(16.dp))
        Text(title, style = MaterialTheme.typography.titleLarge, textAlign = TextAlign.Center)
        if (body != null) {
            Spacer(Modifier.height(8.dp))
            Text(body, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center, modifier = Modifier.widthIn(max = 420.dp))
        }
        if (action != null && onAction != null) {
            Spacer(Modifier.height(20.dp))
            Button(onClick = onAction) { Text(action) }
        }
    }
}

@Composable
fun ErrorState(message: String, onRetry: () -> Unit, modifier: Modifier = Modifier) =
    MessageState(Icons.Rounded.CloudOff, "Couldn't load", message, "Try again", onRetry, modifier)

@Composable
fun SectionHeader(text: String, modifier: Modifier = Modifier, trailing: @Composable (() -> Unit)? = null) {
    Row(modifier.fillMaxWidth().padding(top = 8.dp, bottom = 4.dp), verticalAlignment = Alignment.CenterVertically) {
        Text(text, style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.primary, modifier = Modifier.weight(1f))
        trailing?.invoke()
    }
}

val AiIcon = Icons.Rounded.AutoAwesome
val DoneIcon = Icons.Rounded.CheckCircle

/** "3 min ago" style text for an RFC 3339 timestamp. */
fun relativeTime(ts: String): String {
    if (ts.isBlank()) return ""
    val t = runCatching { Instant.parse(ts).toEpochMilli() }.getOrNull() ?: return ts.take(10)
    return DateUtils.getRelativeTimeSpanString(t, System.currentTimeMillis(), DateUtils.MINUTE_IN_MILLIS, DateUtils.FORMAT_ABBREV_RELATIVE).toString()
}

fun formatDuration(ms: Long): String {
    val s = ms / 1000
    return when {
        s < 60 -> "${s}s"
        s < 3600 -> "${s / 60}m ${s % 60}s"
        else -> "${s / 3600}h ${(s % 3600) / 60}m"
    }
}

val ListPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp)
