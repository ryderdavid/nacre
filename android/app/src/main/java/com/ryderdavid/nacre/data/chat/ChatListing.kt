package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.Connection
import com.ryderdavid.nacre.data.HostState
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.Features
import com.ryderdavid.nacre.data.model.Health

/** The Chat list across every paired computer, and what to say about computers that can't chat. */
data class ChatListing(
    /** Every chat on every computer that serves Chat, newest activity first. */
    val chats: List<ChatRecord> = emptyList(),
    /** One line per connected computer without Chat, e.g. "Update nacred on studio to use Chat". */
    val notices: List<String> = emptyList(),
) {
    fun chat(key: String): ChatRecord? = chats.firstOrNull { it.key == key }
}

/** What the Chat list says about a computer whose nacred predates Chat. */
fun updateForChat(computer: String) = "Update nacred on $computer to use Chat"

/** Whether [health]'s computer serves Chat. */
fun servesChat(health: Health?): Boolean = health != null && Features.AGENT_CHAT in health.features

/**
 * Why a connected computer can't chat, or null when it can. A nacred that knows Chat but can't run it
 * (for example, Claude Code too old for host prompts) gives its own reason.
 */
fun chatBlocker(computer: String, health: Health): String? = when {
    servesChat(health) -> null
    health.agentChatUnavailable.isNotBlank() -> "Chat isn't available on $computer: ${health.agentChatUnavailable}"
    else -> updateForChat(computer)
}

/** Why Codex can't be chosen for a chat on [computer], or null when it can. */
fun codexChatBlocker(computer: String, health: Health?): String? = when {
    health == null -> "$computer isn't connected."
    Features.AGENT_CHAT_CODEX in health.features -> null
    health.agentChatCodexUnavailable.isNotBlank() -> health.agentChatCodexUnavailable
    health.codexUnavailable.isNotBlank() -> health.codexUnavailable
    else -> "Codex chats aren't available on $computer yet. Update nacred there to use Codex in Chat."
}

/**
 * Merges each computer's chat list into one, newest activity first. Each chat is tagged with the
 * paired computer it came from. Computers that are connected but don't serve Chat get a notice
 * instead; computers not connected are left out until they are.
 */
fun mergeChatLists(hosts: List<HostState>, lists: Map<String, List<ChatRecord>>): ChatListing {
    val chats = mutableListOf<ChatRecord>()
    val notices = mutableListOf<String>()
    for (h in hosts) {
        val health = (h.connection as? Connection.Online)?.health ?: continue
        val name = h.host.name.ifBlank { health.hostName.ifBlank { "this computer" } }
        val blocked = chatBlocker(name, health)
        if (blocked != null) {
            notices += blocked
            continue
        }
        lists[h.host.id].orEmpty().forEach { chats += it.copy(hostId = h.host.id, hostName = name) }
    }
    return ChatListing(chats.sortedByDescending { it.updatedAt.ifBlank { it.createdAt } }, notices)
}

/** The Chat tab's badge: chats waiting for a permission answer. */
fun chatBadgeCount(chats: List<ChatRecord>): Int = chats.count { it.state == ChatWire.WAITING_PERMISSION }
