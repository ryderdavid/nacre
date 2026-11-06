package com.ryderdavid.nacre.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.MaterialExpressiveTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.MotionScheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.remember
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import com.google.android.material.color.MaterialColors
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.data.settings.Palette
import com.ryderdavid.nacre.data.settings.ThemeMode

// Fallback palette (no dynamic color): a cool mother-of-pearl teal with lilac and warm accents.
internal val PearlLight = lightColorScheme(
    primary = Color(0xFF00696E), onPrimary = Color.White,
    primaryContainer = Color(0xFF9CF0F5), onPrimaryContainer = Color(0xFF004F53),
    secondary = Color(0xFF615B7E), onSecondary = Color.White,
    secondaryContainer = Color(0xFFE7DEFF), onSecondaryContainer = Color(0xFF494365),
    tertiary = Color(0xFF8E4E2E), onTertiary = Color.White,
    tertiaryContainer = Color(0xFFFFDBCC), onTertiaryContainer = Color(0xFF713719),
    background = Color(0xFFF7F9FA), onBackground = Color(0xFF181C1D),
    surface = Color(0xFFF7F9FA), onSurface = Color(0xFF181C1D),
    surfaceVariant = Color(0xFFDAE4E5), onSurfaceVariant = Color(0xFF3F4849),
    surfaceContainerLowest = Color.White, surfaceContainerLow = Color(0xFFF1F4F4),
    surfaceContainer = Color(0xFFEBEEEF), surfaceContainerHigh = Color(0xFFE6E9E9),
    surfaceContainerHighest = Color(0xFFE0E3E3),
    outline = Color(0xFF6F797A), outlineVariant = Color(0xFFBEC8C9),
)

internal val PearlDark = darkColorScheme(
    primary = Color(0xFF80D4D9), onPrimary = Color(0xFF00373A),
    primaryContainer = Color(0xFF004F53), onPrimaryContainer = Color(0xFF9CF0F5),
    secondary = Color(0xFFCBC2EA), onSecondary = Color(0xFF322D4D),
    secondaryContainer = Color(0xFF494365), onSecondaryContainer = Color(0xFFE7DEFF),
    tertiary = Color(0xFFFFB693), onTertiary = Color(0xFF552106),
    tertiaryContainer = Color(0xFF713719), onTertiaryContainer = Color(0xFFFFDBCC),
    background = Color(0xFF0F1415), onBackground = Color(0xFFDEE3E3),
    surface = Color(0xFF0F1415), onSurface = Color(0xFFDEE3E3),
    surfaceVariant = Color(0xFF3F4849), onSurfaceVariant = Color(0xFFBEC8C9),
    surfaceContainerLowest = Color(0xFF0A0F10), surfaceContainerLow = Color(0xFF181C1D),
    surfaceContainer = Color(0xFF1C2021), surfaceContainerHigh = Color(0xFF262B2B),
    surfaceContainerHighest = Color(0xFF313536),
    outline = Color(0xFF889393), outlineVariant = Color(0xFF3F4849),
)

/** Semantic colors that are not part of the M3 scheme, harmonized toward its primary. */
@Immutable
data class NacreColors(
    val ready: Color,
    val doing: Color,
    val blocked: Color,
    val later: Color,
    val done: Color,
    /** Waiting only on Ryder's sign-off. */
    val check: Color,
    val needsYou: Color,
    val needsYouContainer: Color,
    val priority: List<Color>,
    val bug: Color,
    val feature: Color,
    val task: Color,
    val epic: Color,
    val decision: Color,
    val chore: Color,
    val running: Color,
    /** Specification state: proposals, phases, sections. */
    val spec: Color,
) {
    fun lane(lane: Lane) = when (lane) {
        Lane.PROPOSED -> spec
        Lane.READY -> ready
        Lane.DOING -> doing
        Lane.BLOCKED -> blocked
        Lane.LATER -> later
        Lane.CHECK -> check
        Lane.DONE -> done
    }

    fun type(type: String) = when (type) {
        "bug" -> bug
        "feature", "story" -> feature
        "epic", "milestone" -> epic
        "decision" -> decision
        "chore" -> chore
        else -> task
    }
}

val LocalNacreColors = staticCompositionLocalOf<NacreColors> { error("NacreColors not provided") }

private fun nacreColors(scheme: ColorScheme, dark: Boolean): NacreColors {
    val p = scheme.primary.toArgb()
    fun h(light: Long, darkC: Long) = Color(MaterialColors.harmonize((if (dark) darkC else light).toInt(), p))
    return NacreColors(
        ready = scheme.primary,
        doing = h(0xFF2E7D4F, 0xFF7CC79B),
        blocked = h(0xFFC2571A, 0xFFF0A160),
        later = scheme.outline,
        done = scheme.onSurfaceVariant,
        check = scheme.tertiary,
        needsYou = h(0xFFB5560F, 0xFFF0A160),
        needsYouContainer = h(0xFFFBEADB, 0xFF3A2616),
        priority = listOf(
            h(0xFFB42318, 0xFFF2867D),
            h(0xFFC2571A, 0xFFF0A160),
            scheme.primary,
            scheme.secondary,
            scheme.outline,
        ),
        bug = h(0xFFB42318, 0xFFF2867D),
        feature = h(0xFF6A4FB3, 0xFFC4B0FF),
        task = h(0xFF2C5F8A, 0xFF7FB0D8),
        epic = h(0xFF9A6A00, 0xFFE9C46A),
        decision = h(0xFFB5560F, 0xFFF0A160),
        chore = scheme.outline,
        running = h(0xFF6A4FB3, 0xFFC4B0FF),
        spec = scheme.secondary,
    )
}

val MonoFamily = FontFamily.Monospace

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun NacreTheme(themeMode: ThemeMode = ThemeMode.SYSTEM, palette: Palette? = null, content: @Composable () -> Unit) {
    val dark = when (themeMode) {
        ThemeMode.SYSTEM -> isSystemInDarkTheme()
        ThemeMode.LIGHT -> false
        ThemeMode.DARK -> true
    }
    val context = LocalContext.current
    val chosen = Palettes.resolve(palette, Palettes.dynamicAvailable)
    val scheme = if (chosen == Palette.WALLPAPER) {
        if (dark) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
    } else {
        Palettes.scheme(chosen, dark)
    }
    val extra = remember(scheme, dark) { nacreColors(scheme, dark) }
    CompositionLocalProvider(LocalNacreColors provides extra) {
        MaterialExpressiveTheme(colorScheme = scheme, motionScheme = MotionScheme.expressive(), typography = NacreTypography, content = content)
    }
}

object Nacre {
    val colors: NacreColors @Composable get() = LocalNacreColors.current
}

/** Convenience for components that only need the M3 scheme. */
val scheme: ColorScheme @Composable get() = MaterialTheme.colorScheme
