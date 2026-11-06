# Spec Delta

## Purpose

Palettes let Ryder choose the app's colors: the phone's wallpaper colors, or one of several fixed
Material 3 palettes. Each palette has light and dark versions that meet the accessibility contract.

## ADDED Requirements

### Requirement: Palette picker
Settings › Appearance SHALL offer a palette picker with:
- **Wallpaper**: dynamic color from the phone, offered only when the device supports it
- **Pearl**: Nacre's mother-of-pearl teal, the default when Wallpaper isn't available
- **Ocean**: blues throughout, with blue primary, secondary and tertiary roles
- **Sage**
- **Ember**
- **Orchid**

#### Scenario: Pick Ocean
- **WHEN** Ryder taps the Ocean swatch
- **THEN** the whole app switches to the Ocean palette at once

### Requirement: Palette swatches
Each palette SHALL show as a swatch of its primary, secondary and tertiary colors. The selected swatch
SHALL show a check and a changed shape, and SHALL be announced as selected.

#### Scenario: Selected swatch
- **WHEN** Ocean is the current palette and TalkBack focuses its swatch
- **THEN** the swatch shows a check and a squarer shape, and Ocean is announced as selected

#### Scenario: No dynamic color
- **WHEN** the device doesn't support dynamic color
- **THEN** Wallpaper isn't offered, and Pearl is selected by default

### Requirement: Palettes are full Material 3 schemes
Each fixed palette SHALL be generated from HCT tonal palettes, with a hue and chroma for each of primary,
secondary, tertiary, neutral and neutral variant. It SHALL define every color role in a light and a dark
version. Every palette SHALL meet the contrast thresholds of the app-accessibility capability in both
versions.

#### Scenario: Contrast test
- **WHEN** the unit tests run
- **THEN** each palette's text and control pairs meet 4.5:1 and 3:1 in light and dark, and the test fails if any pair doesn't

### Requirement: Palette and theme are remembered
The chosen palette and the theme (System, Light or Dark) SHALL persist across restarts. Each SHALL apply
to every screen without a restart.

#### Scenario: Restart
- **WHEN** Ryder picks Sage with Dark and restarts the app
- **THEN** the app opens in Sage, dark
