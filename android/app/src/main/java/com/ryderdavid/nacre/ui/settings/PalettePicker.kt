package com.ryderdavid.nacre.ui.settings

import androidx.compose.animation.core.animateDpAsState
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.settings.Palette
import com.ryderdavid.nacre.ui.theme.Palettes

/**
 * The palette picker: one swatch per palette, showing its primary (top half), tertiary (bottom right) and
 * secondary (bottom left) colors. The selected swatch turns from a circle into a rounded square, gains a
 * ring and a check, and is announced as selected.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun PalettePicker(current: Palette, dynamicAvailable: Boolean, onPick: (Palette) -> Unit, modifier: Modifier = Modifier) {
    FlowRow(
        modifier.selectableGroup(),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
        maxItemsInEachRow = 3,
    ) {
        Palettes.offered(dynamicAvailable).forEach { p ->
            PaletteSwatch(p, selected = p == current, onClick = { onPick(p) }, modifier = Modifier.weight(1f))
        }
    }
}

@Composable
private fun PaletteSwatch(palette: Palette, selected: Boolean, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val ring by animateDpAsState(if (selected) 26.dp else 38.dp, MaterialTheme.motionScheme.fastSpatialSpec(), label = "ring")
    val inner by animateDpAsState(if (selected) 20.dp else 30.dp, MaterialTheme.motionScheme.fastSpatialSpec(), label = "swatch")
    val label = palette.label + if (palette == Palette.PEARL) " (default)" else ""
    Column(
        modifier
            .selectable(selected = selected, onClick = onClick, role = Role.RadioButton)
            .semantics { contentDescription = "$label palette" }
            .padding(vertical = 8.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Box(Modifier.size(76.dp), contentAlignment = Alignment.Center) {
            Box(
                Modifier.size(76.dp).clip(RoundedCornerShape(ring))
                    .then(if (selected) Modifier.border(3.dp, MaterialTheme.colorScheme.primary, RoundedCornerShape(ring)) else Modifier),
                contentAlignment = Alignment.Center,
            ) {
                SwatchFill(Palettes.swatch(palette), wallpaper = palette == Palette.WALLPAPER, Modifier.size(60.dp).clip(RoundedCornerShape(inner)))
            }
            if (selected) {
                Box(
                    Modifier.align(Alignment.BottomEnd).size(26.dp).clip(CircleShape)
                        .background(MaterialTheme.colorScheme.surface).padding(3.dp)
                        .clip(CircleShape).background(MaterialTheme.colorScheme.primary),
                    contentAlignment = Alignment.Center,
                ) { Icon(Icons.Rounded.Check, null, Modifier.size(16.dp), tint = MaterialTheme.colorScheme.onPrimary) }
            }
        }
        Text(label, style = MaterialTheme.typography.labelLarge, textAlign = TextAlign.Center, modifier = Modifier.width(96.dp))
    }
}

@Composable
private fun SwatchFill(colors: List<Color>, wallpaper: Boolean, modifier: Modifier) {
    val (primary, secondary, tertiary) = colors
    Canvas(modifier) {
        if (wallpaper) {
            drawRect(Brush.sweepGradient(listOf(primary, secondary, tertiary, primary)))
            return@Canvas
        }
        drawRect(primary, size = Size(size.width, size.height / 2))
        drawRect(secondary, topLeft = Offset(0f, size.height / 2), size = Size(size.width / 2, size.height / 2))
        drawRect(tertiary, topLeft = Offset(size.width / 2, size.height / 2), size = Size(size.width / 2, size.height / 2))
    }
}
