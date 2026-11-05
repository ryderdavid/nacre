package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.Connection
import com.ryderdavid.nacre.data.HostState
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.Health
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.Repo

/** Where a new chat runs: the computer of the screen it starts from, and its repository there. */
data class NewChatTarget(
    val hostId: String,
    val computer: String,
    /** Repository id on [hostId]; null for all boards. */
    val repoId: String?,
    /** The repository's name, or "all boards". */
    val label: String,
    /** Why a chat can't start there (offline, or no Chat), or null when it can. */
    val blocker: String? = null,
    val health: Health? = null,
)

/**
 * The computer the across-boards chat runs on: of the computers serving Chat, the one holding
 * the most boards, earlier in pairing order on a tie.
 */
fun acrossBoardsHost(hosts: List<HostState>, repos: List<Repo>): String? {
    val serving = hosts.filter { servesChat((it.connection as? Connection.Online)?.health) }.map { it.host.id }
    val pool = serving.ifEmpty { hosts.map { it.host.id } }
    if (pool.isEmpty()) return null
    val rank = pool.withIndex().associate { (i, h) -> h to i }
    return pool.sortedWith(compareByDescending<String> { h -> repos.count { it.hostId == h } }.thenBy { rank[it] }).first()
}

/**
 * Where New starts a chat for [shown]: its repository's computer for a board, bead, change, round
 * or run, or the across-boards computer for Boards, Inbox and Activity. [forceRepoKey] picks a
 * repository's own computer, for "New chat on <computer>".
 */
fun newChatTarget(shown: ShownScreen, hosts: List<HostState>, repos: List<Repo>, forceRepoKey: String? = null): NewChatTarget? {
    val repoKey = forceRepoKey ?: shown.repoKey
    val hostId = repoKey?.let(Keys::host) ?: acrossBoardsHost(hosts, repos) ?: return null
    val hs = hosts.firstOrNull { it.host.id == hostId }
    val computer = hs?.host?.name?.ifBlank { null } ?: "this computer"
    val health = (hs?.connection as? Connection.Online)?.health
    val blocker = when (val c = hs?.connection) {
        null -> "That computer is no longer paired."
        is Connection.Online -> chatBlocker(computer, c.health)
        is Connection.Offline -> "$computer is offline: ${c.reason}"
        else -> "Connecting to $computer…"
    }
    val label = repoKey?.let { k -> repos.firstOrNull { it.key == k }?.name ?: Keys.id(k) } ?: com.ryderdavid.nacre.ui.ScreenOrigin.ALL_BOARDS
    return NewChatTarget(hostId, computer, repoKey?.let(Keys::id), label, blocker, health)
}

/** One agent in the New chat sheet. */
data class AgentOption(val agent: String, val label: String, val enabled: Boolean, val reason: String? = null, val note: String? = null)

/** What the New chat sheet says about Codex. */
const val CODEX_NOTE = "Codex has no permission classifier and no USD runaway limit. Your ChatGPT plan's own usage limits apply."

/** Claude and Codex for a chat on [target]'s computer; Codex is disabled with nacred's reason when it can't serve chats. */
fun agentOptions(target: NewChatTarget): List<AgentOption> {
    val codex = codexChatBlocker(target.computer, target.health)
    return listOf(
        AgentOption(ChatWire.AGENT_CLAUDE, "Claude", enabled = true),
        AgentOption(ChatWire.AGENT_CODEX, "Codex", enabled = codex == null, reason = codex, note = CODEX_NOTE),
    )
}

/** The agent to preselect: the remembered one, unless it is disabled here. */
fun preselectedAgent(remembered: String, options: List<AgentOption>): String =
    options.firstOrNull { it.agent == remembered && it.enabled }?.agent ?: ChatWire.AGENT_CLAUDE
