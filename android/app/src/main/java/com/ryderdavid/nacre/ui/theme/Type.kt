package com.ryderdavid.nacre.ui.theme

import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.Typography
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontVariation
import androidx.compose.ui.text.font.FontWeight
import com.ryderdavid.nacre.R

/** Roboto Flex (bundled, SIL OFL) at a given width: weights 300–900 from the one variable file. */
private fun flex(width: Float): FontFamily = FontFamily(
    listOf(300, 400, 500, 600, 700, 800, 900).map { w ->
        Font(R.font.roboto_flex, weight = FontWeight(w), variationSettings = FontVariation.Settings(FontVariation.weight(w), FontVariation.width(width)))
    },
)

private val Regular = flex(100f)

/** Emphasized styles: a touch wider. */
private val Wide = flex(112f)

private fun TextStyle.regular() = copy(fontFamily = Regular)
private fun TextStyle.emphasized() = copy(fontFamily = Wide, fontWeight = FontWeight((fontWeight?.weight ?: 400).coerceAtLeast(600) + 100), letterSpacing = letterSpacing)

/** M3 Expressive type scale in Roboto Flex; the emphasized variants are heavier and wider. */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
val NacreTypography: Typography = Typography().let { b ->
    Typography(
        displayLarge = b.displayLarge.regular(), displayMedium = b.displayMedium.regular(), displaySmall = b.displaySmall.regular(),
        headlineLarge = b.headlineLarge.regular(), headlineMedium = b.headlineMedium.regular(), headlineSmall = b.headlineSmall.regular(),
        titleLarge = b.titleLarge.regular(), titleMedium = b.titleMedium.regular(), titleSmall = b.titleSmall.regular(),
        bodyLarge = b.bodyLarge.regular(), bodyMedium = b.bodyMedium.regular(), bodySmall = b.bodySmall.regular(),
        labelLarge = b.labelLarge.regular(), labelMedium = b.labelMedium.regular(), labelSmall = b.labelSmall.regular(),
        displayLargeEmphasized = b.displayLargeEmphasized.emphasized(), displayMediumEmphasized = b.displayMediumEmphasized.emphasized(),
        displaySmallEmphasized = b.displaySmallEmphasized.emphasized(),
        headlineLargeEmphasized = b.headlineLargeEmphasized.emphasized(), headlineMediumEmphasized = b.headlineMediumEmphasized.emphasized(),
        headlineSmallEmphasized = b.headlineSmallEmphasized.emphasized(),
        titleLargeEmphasized = b.titleLargeEmphasized.emphasized(), titleMediumEmphasized = b.titleMediumEmphasized.emphasized(),
        titleSmallEmphasized = b.titleSmallEmphasized.emphasized(),
        bodyLargeEmphasized = b.bodyLargeEmphasized.regular(), bodyMediumEmphasized = b.bodyMediumEmphasized.regular(), bodySmallEmphasized = b.bodySmallEmphasized.regular(),
        labelLargeEmphasized = b.labelLargeEmphasized.regular(), labelMediumEmphasized = b.labelMediumEmphasized.regular(), labelSmallEmphasized = b.labelSmallEmphasized.regular(),
    )
}
