package com.ryderdavid.nacre.ui.theme

import android.os.Build
import androidx.compose.material3.ColorScheme
import androidx.compose.ui.graphics.Color
import com.ryderdavid.nacre.data.settings.Palette

/** The fixed palettes as color schemes, and which palette applies. */
object Palettes {
    /** Dynamic color needs Android 12 (S) or later. */
    val dynamicAvailable: Boolean get() = Build.VERSION.SDK_INT >= Build.VERSION_CODES.S

    /** The palettes the picker offers: Wallpaper only where dynamic color exists. */
    fun offered(dynamic: Boolean): List<Palette> = Palette.entries.filter { it != Palette.WALLPAPER || dynamic }

    /** The palette in use: the chosen one if it can apply, else Wallpaper where available, else Pearl. */
    fun resolve(chosen: Palette?, dynamic: Boolean): Palette = when {
        chosen == Palette.WALLPAPER && !dynamic -> Palette.PEARL
        chosen != null -> chosen
        dynamic -> Palette.WALLPAPER
        else -> Palette.PEARL
    }

    /** The scheme of a fixed palette. Wallpaper has none here: it comes from the system. */
    fun scheme(palette: Palette, dark: Boolean): ColorScheme = when (palette) {
        Palette.OCEAN -> if (dark) OceanDark else OceanLight
        Palette.SAGE -> if (dark) SageDark else SageLight
        Palette.EMBER -> if (dark) EmberDark else EmberLight
        Palette.ORCHID -> if (dark) OrchidDark else OrchidLight
        Palette.PEARL, Palette.WALLPAPER -> if (dark) PearlDark else PearlLight
    }

    /** Primary, secondary and tertiary colors for a palette's swatch. Wallpaper shows a stand-in blend. */
    fun swatch(palette: Palette): List<Color> = when (palette) {
        Palette.WALLPAPER -> listOf(Color(0xFF5E7C3A), Color(0xFFB79B64), Color(0xFF7C9AC0))
        else -> GeneratedSwatches.getValue(palette.label)
    }
}
