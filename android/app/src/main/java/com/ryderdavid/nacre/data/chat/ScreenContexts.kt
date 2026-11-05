package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.RepoGroup
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.ScreenContext
import com.ryderdavid.nacre.ui.ScreenOrigin
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/**
 * What one screen says about itself while it is shown: the context sent with a message, the
 * repository key (with its computer) the context is about, and the label for "About: <label>".
 */
data class ShownScreen(
    val context: ScreenContext,
    /** "<host>~<repo id>" of the repository shown, or null for all boards. */
    val repoKey: String? = null,
    val label: String,
)

/** A stable key for a screen, ignoring display names that may load later. */
fun originKey(origin: ScreenOrigin): String = when (origin) {
    is ScreenOrigin.AllBoards -> "all:${origin.screen}"
    is ScreenOrigin.Board -> "board:${origin.repoKey}"
    is ScreenOrigin.Bead -> "bead:${origin.repoKey}:${origin.beadId}"
    is ScreenOrigin.Change -> "change:${origin.repoKey}:${origin.beadId}"
    is ScreenOrigin.Round -> "round:${origin.roundKey}"
    is ScreenOrigin.Run -> "run:${origin.runKey}"
}

/** What a screen is known to be before it has published anything: its kind, ids and label. */
fun bareScreen(origin: ScreenOrigin?): ShownScreen = when (origin) {
    null -> ShownScreen(ScreenContext(ScreenContext.BOARDS), null, ScreenOrigin.ALL_BOARDS)
    is ScreenOrigin.AllBoards -> ShownScreen(ScreenContext(origin.screen), null, origin.label)
    is ScreenOrigin.Board -> ShownScreen(ScreenContext(ScreenContext.BOARD, repo = Keys.id(origin.repoKey)), origin.repoKey, origin.label)
    is ScreenOrigin.Bead -> ShownScreen(ScreenContext(ScreenContext.BEAD, repo = Keys.id(origin.repoKey), beadId = origin.beadId), origin.repoKey, origin.label)
    is ScreenOrigin.Change -> ShownScreen(ScreenContext(ScreenContext.CHANGE, repo = Keys.id(origin.repoKey), beadId = origin.beadId), origin.repoKey, origin.label)
    is ScreenOrigin.Round -> ShownScreen(ScreenContext(ScreenContext.ROUND, roundId = Keys.id(origin.roundKey)), null, origin.label)
    is ScreenOrigin.Run -> ShownScreen(ScreenContext(ScreenContext.RUN, runId = Keys.id(origin.runKey)), null, origin.label)
}

/** [text] cut to at most [maxBytes] of UTF-8, never splitting a character. */
fun trimUtf8(text: String, maxBytes: Int = ScreenContext.SUMMARY_MAX_BYTES): String {
    if (text.toByteArray(Charsets.UTF_8).size <= maxBytes) return text
    val ellipsis = "…"
    val budget = maxBytes - ellipsis.toByteArray(Charsets.UTF_8).size
    var bytes = 0
    var end = 0
    while (end < text.length) {
        val cp = text.codePointAt(end)
        val n = String(Character.toChars(cp)).toByteArray(Charsets.UTF_8).size
        if (bytes + n > budget) break
        bytes += n
        end += Character.charCount(cp)
    }
    return text.substring(0, end) + ellipsis
}

/** Builds a screen summary from labelled lines, dropping blanks, within the 4 KB limit. */
fun summaryOf(vararg lines: Pair<String, String?>): String =
    trimUtf8(lines.mapNotNull { (k, v) -> v?.trim()?.takeIf { it.isNotEmpty() }?.let { if (k.isEmpty()) it else "$k: $it" } }.joinToString("\n"))

/**
 * The one screen-context holder. Board, bead, change, round, run, Boards, Inbox and Activity
 * [publish] what they show; the shell says which screen is the one Chat is about with [show]
 * (the screen shown, or under Chat the screen last shown before it). Send reads [atSend].
 */
class ScreenContextHolder {
    private val published = MutableStateFlow<Map<String, ShownScreen>>(emptyMap())
    private val about = MutableStateFlow<ScreenOrigin?>(null)
    private val currentState = MutableStateFlow(bareScreen(null))

    /** The screen the next message is about; it changes as Ryder moves. */
    val current: StateFlow<ShownScreen> = currentState.asStateFlow()

    /** The screen Chat is about now: the one shown, or the last one before Chat. */
    fun show(origin: ScreenOrigin?) {
        about.value = origin
        recompute()
    }

    /** A screen's own account of itself, replacing what it said before. */
    fun publish(origin: ScreenOrigin, screen: ShownScreen) {
        val trimmed = screen.copy(context = screen.context.copy(summary = screen.context.summary?.let(::trimUtf8)))
        published.update { it + (originKey(origin) to trimmed) }
        recompute()
    }

    /** What to send with a message: read at the moment Send is pressed. */
    fun atSend(): ShownScreen = currentState.value

    @Synchronized private fun recompute() {
        val origin = about.value
        currentState.value = origin?.let { published.value[originKey(it)] } ?: bareScreen(origin)
    }
}

/** A message's context mapped onto the chat's computer, and where the repository is when it is elsewhere. */
data class SendContext(val context: ScreenContext, val label: String, val elsewhere: Elsewhere? = null)

/** The screen's repository has no checkout on the chat's computer: it is on [hostId] ([computer]). */
data class Elsewhere(val repoName: String, val hostId: String, val computer: String, val repoKey: String)

/**
 * Maps [shown]'s repository onto the chat's computer [chatHostId]: its own key when it lives
 * there, the same repository's checkout there through its repo group, or else the key on its own
 * computer marked `elsewhere`.
 */
fun contextForChat(
    shown: ShownScreen,
    chatHostId: String,
    groups: List<RepoGroup>,
    hostName: (String) -> String,
): SendContext {
    val key = shown.repoKey ?: return SendContext(shown.context.copy(repo = null, elsewhere = null), shown.label)
    val host = Keys.host(key)
    if (host == chatHostId) return SendContext(shown.context.copy(repo = Keys.id(key), elsewhere = null), shown.label)
    val group = groups.firstOrNull { g -> g.members.any { it.key == key } }
    group?.members?.firstOrNull { it.hostId == chatHostId || Keys.host(it.key) == chatHostId }?.let { local ->
        return SendContext(shown.context.copy(repo = Keys.id(local.key), elsewhere = null), shown.label)
    }
    val computer = hostName(host).ifBlank { "another computer" }
    val name = group?.members?.firstOrNull { it.key == key }?.name ?: Keys.id(key)
    return SendContext(
        shown.context.copy(repo = Keys.id(key), elsewhere = computer),
        shown.label,
        Elsewhere(name, host, computer, key),
    )
}

/** The composer's hint when the screen's repository is on another computer. */
fun elsewhereHint(chatComputer: String, e: Elsewhere) = "This chat runs on $chatComputer. ${e.repoName} is on ${e.computer}."

/** The composer's chip. */
fun aboutChip(label: String) = "About: $label"
