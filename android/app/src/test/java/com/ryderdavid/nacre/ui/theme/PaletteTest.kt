package com.ryderdavid.nacre.ui.theme

import androidx.compose.material3.ColorScheme
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.luminance
import com.ryderdavid.nacre.data.settings.Palette
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PaletteTest {
    private fun contrast(a: Color, b: Color): Double {
        val la = a.luminance().toDouble()
        val lb = b.luminance().toDouble()
        return (maxOf(la, lb) + 0.05) / (minOf(la, lb) + 0.05)
    }

    /** The app-accessibility thresholds: 4.5:1 for text, 3:1 for controls and state indicators. */
    private fun pairs(s: ColorScheme) = listOf(
        Triple("onSurface on surface", s.onSurface to s.surface, 4.5),
        Triple("onSurfaceVariant on surfaceContainerHighest", s.onSurfaceVariant to s.surfaceContainerHighest, 4.5),
        Triple("onSurface on surfaceContainerHighest", s.onSurface to s.surfaceContainerHighest, 4.5),
        Triple("onPrimary on primary", s.onPrimary to s.primary, 4.5),
        Triple("onPrimaryContainer on primaryContainer", s.onPrimaryContainer to s.primaryContainer, 4.5),
        Triple("onSecondary on secondary", s.onSecondary to s.secondary, 4.5),
        Triple("onSecondaryContainer on secondaryContainer", s.onSecondaryContainer to s.secondaryContainer, 4.5),
        Triple("onTertiaryContainer on tertiaryContainer", s.onTertiaryContainer to s.tertiaryContainer, 4.5),
        Triple("secondary on surfaceContainer", s.secondary to s.surfaceContainer, 4.5),
        Triple("outline on surfaceContainerHigh", s.outline to s.surfaceContainerHigh, 3.0),
        Triple("primary on surface", s.primary to s.surface, 3.0),
        Triple("inverseOnSurface on inverseSurface", s.inverseOnSurface to s.inverseSurface, 4.5),
    )

    @Test
    fun `every fixed palette meets the contrast contract in light and dark`() {
        val failures = mutableListOf<String>()
        for (p in Palette.entries.filter { it != Palette.WALLPAPER }) for (dark in listOf(false, true)) {
            for ((name, colors, need) in pairs(Palettes.scheme(p, dark))) {
                val c = contrast(colors.first, colors.second)
                if (c < need) failures += "%s %s: %s is %.2f, needs %.1f".format(p, if (dark) "dark" else "light", name, c, need)
            }
        }
        assertTrue(failures.joinToString("\n"), failures.isEmpty())
    }

    @Test
    fun `ocean is blue throughout`() {
        for (dark in listOf(false, true)) {
            val s = Palettes.scheme(Palette.OCEAN, dark)
            for (c in listOf(s.primary, s.secondary, s.tertiary)) {
                assertTrue("$c is not blue", c.blue > c.red && c.blue >= c.green)
            }
        }
    }

    @Test
    fun `the default is wallpaper where dynamic color exists, else pearl`() {
        assertEquals(Palette.WALLPAPER, Palettes.resolve(null, dynamic = true))
        assertEquals(Palette.PEARL, Palettes.resolve(null, dynamic = false))
        assertEquals(Palette.PEARL, Palettes.resolve(Palette.WALLPAPER, dynamic = false))
        assertEquals(Palette.OCEAN, Palettes.resolve(Palette.OCEAN, dynamic = true))
    }

    @Test
    fun `wallpaper is offered only with dynamic color`() {
        assertFalse(Palette.WALLPAPER in Palettes.offered(dynamic = false))
        assertTrue(Palette.WALLPAPER in Palettes.offered(dynamic = true))
        assertEquals(6, Palettes.offered(dynamic = true).size)
    }

    @Test
    fun `a stored palette name round-trips and an unknown one falls back`() {
        for (p in Palette.entries) assertEquals(p, Palette.parse(p.name))
        assertEquals(null, Palette.parse("NEON"))
        assertEquals(null, Palette.parse(null))
    }
}
