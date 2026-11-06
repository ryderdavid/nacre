package com.ryderdavid.nacre.ui.workspace.spec

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp

/** A status label: always text (and optionally an icon), never color alone. */
@Composable
internal fun SpecPill(text: String, container: Color, content: Color, icon: ImageVector? = null, modifier: Modifier = Modifier) {
    Surface(color = container, contentColor = content, shape = RoundedCornerShape(12.dp), modifier = modifier) {
        Row(Modifier.padding(horizontal = 10.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            if (icon != null) Icon(icon, null, Modifier.size(16.dp))
            Text(text, style = MaterialTheme.typography.labelMedium)
        }
    }
}

/** A group or section heading inside the pane, announced as a heading. */
@Composable
internal fun PaneHeading(text: String, modifier: Modifier = Modifier, trailing: (@Composable () -> Unit)? = null) {
    Row(modifier.fillMaxWidth().padding(start = 8.dp, end = 8.dp, top = 12.dp, bottom = 4.dp), verticalAlignment = Alignment.Bottom) {
        Text(text, style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f).semantics { heading() })
        trailing?.invoke()
    }
}

/**
 * Section progress as a segmented bar: landed, verified, in a round, failed, waiting. The bar is
 * decoration; TalkBack reads [SectionProgress.text], and the same words are shown beside it.
 */
@Composable
internal fun ProgressTrack(p: SectionProgress, modifier: Modifier = Modifier, height: Int = 6) {
    val c = MaterialTheme.colorScheme
    val parts = listOf(p.landed to c.primary, p.verified to c.secondary, p.working to c.tertiary, p.failed to c.error, p.waiting to c.surfaceContainerHighest)
    Row(modifier.fillMaxWidth().height(height.dp).clearAndSetSemantics { contentDescription = p.text }, horizontalArrangement = Arrangement.spacedBy(3.dp)) {
        parts.filter { it.first > 0 }.forEach { (n, color) ->
            Box(Modifier.weight(n.toFloat()).height(height.dp).background(color, RoundedCornerShape(height.dp)))
        }
    }
}

/** The pane's top bar: Back (when deeper than the list), a title, a subtitle and actions. */
@Composable
internal fun PaneTopBar(
    title: String,
    subtitle: String,
    onBack: (() -> Unit)?,
    backLabel: String = "Back",
    actions: @Composable RowScope.() -> Unit = {},
) {
    Row(Modifier.fillMaxWidth().heightIn(min = 64.dp).padding(start = 4.dp, end = 4.dp), verticalAlignment = Alignment.CenterVertically) {
        if (onBack != null) IconButton(onClick = onBack, modifier = Modifier.size(48.dp)) { Icon(Icons.AutoMirrored.Rounded.ArrowBack, backLabel) }
        Column(Modifier.weight(1f).padding(horizontal = if (onBack == null) 16.dp else 8.dp, vertical = 6.dp)) {
            Text(title, style = MaterialTheme.typography.titleLargeEmphasized, modifier = Modifier.semantics { heading() })
            if (subtitle.isNotBlank()) Text(subtitle, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        actions()
    }
}

/** A pane search field (search app bar style) that takes focus when it opens. */
@Composable
internal fun PaneSearchField(
    value: String,
    onValue: (String) -> Unit,
    placeholder: String,
    onClose: () -> Unit,
    modifier: Modifier = Modifier,
    trailing: (@Composable RowScope.() -> Unit)? = null,
) {
    val focus = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }
    TextField(
        value = value, onValueChange = onValue,
        modifier = modifier.fillMaxWidth().heightIn(min = 56.dp).focusRequester(focus),
        placeholder = { Text(placeholder) },
        leadingIcon = { Icon(Icons.Rounded.Search, null) },
        trailingIcon = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                trailing?.invoke(this)
                IconButton(onClick = onClose, modifier = Modifier.size(48.dp)) { Icon(Icons.Rounded.Close, "Close search") }
            }
        },
        singleLine = true,
        shape = RoundedCornerShape(28.dp),
        colors = TextFieldDefaults.colors(
            focusedIndicatorColor = Color.Transparent,
            unfocusedIndicatorColor = Color.Transparent,
            unfocusedContainerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
            focusedContainerColor = MaterialTheme.colorScheme.surfaceContainerHighest,
        ),
    )
}
