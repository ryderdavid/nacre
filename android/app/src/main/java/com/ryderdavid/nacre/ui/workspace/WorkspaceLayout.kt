package com.ryderdavid.nacre.ui.workspace

/** The workspace's three destinations. */
enum class Destination(val label: String) { CHAT("Chat"), SPEC("Spec"), BEADS("Beads") }

/** How the workspace is laid out for a window. */
sealed interface WorkspaceLayout {
    /** One destination at a time, with the short navigation bar (or a rail at compact height). */
    data class Single(val rail: Boolean) : WorkspaceLayout

    /** Chat beside Spec or Beads; [chatDp] and [paneDp] add up to the usable width. */
    data class Split(val chatDp: Int, val paneDp: Int) : WorkspaceLayout
}

/** The divider presets. */
enum class PanePreset(val label: String) { CHAT_WIDE("Chat wide"), EVEN("Even"), PANE_WIDE("Pane wide") }

/**
 * Layout rules from the app-navigation spec. Widths in dp. The rail is 96dp and the divider 16dp; Chat is at
 * least 360dp and the supporting pane at least 320dp, so two panes need a usable width of 680dp.
 */
object WorkspaceRules {
    const val RAIL = 96
    const val DIVIDER = 16
    const val CHAT_MIN = 360
    const val PANE_MIN = 320
    const val SPLIT_MIN = CHAT_MIN + PANE_MIN
    const val COMPACT_WIDTH = 600
    const val COMPACT_HEIGHT = 480

    /** Whether the window gets the rail (wider than compact) or the bottom bar. */
    fun usesRail(widthDp: Int, heightDp: Int): Boolean = widthDp >= COMPACT_WIDTH || heightDp < COMPACT_HEIGHT

    /** The usable width beside the rail: the window minus the rail and the divider. */
    fun usable(widthDp: Int): Int = widthDp - RAIL - DIVIDER

    /** Chat's width after Reset: half the usable width, clamped to both minimums. */
    fun reset(usable: Int): Int = (usable / 2).coerceIn(CHAT_MIN, usable - PANE_MIN)

    /** Chat's width for a preset, or null when that preset would break a minimum (then it isn't offered). */
    fun preset(p: PanePreset, usable: Int): Int? {
        if (usable < SPLIT_MIN) return null
        val chat = when (p) {
            PanePreset.CHAT_WIDE -> usable - PANE_MIN
            PanePreset.EVEN -> usable / 2
            PanePreset.PANE_WIDE -> CHAT_MIN
        }
        return chat.takeIf { it >= CHAT_MIN && usable - it >= PANE_MIN }
    }

    /** The presets that keep both minimums at this usable width. */
    fun offered(usable: Int): List<PanePreset> = PanePreset.entries.filter { preset(it, usable) != null }

    /** A dragged chat width, clamped to the minimums. */
    fun clamp(chatDp: Int, usable: Int): Int = chatDp.coerceIn(CHAT_MIN, (usable - PANE_MIN).coerceAtLeast(CHAT_MIN))

    /**
     * The layout for a window showing [destination]. With Chat alone, or without room for two panes, one
     * destination shows. [chatDp] is the remembered chat width, or null for Reset.
     */
    fun layout(widthDp: Int, heightDp: Int, destination: Destination, chatAlone: Boolean, chatDp: Int?): WorkspaceLayout {
        if (!usesRail(widthDp, heightDp)) return WorkspaceLayout.Single(rail = false)
        val usable = usable(widthDp)
        if (chatAlone || destination == Destination.CHAT || usable < SPLIT_MIN) return WorkspaceLayout.Single(rail = true)
        val chat = clamp(chatDp ?: reset(usable), usable)
        return WorkspaceLayout.Split(chat, usable - chat)
    }
}
