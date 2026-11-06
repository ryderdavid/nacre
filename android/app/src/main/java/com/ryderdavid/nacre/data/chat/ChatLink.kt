package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.bridge.ChatStreamMessage
import com.ryderdavid.nacre.data.model.ChatEvent
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.model.ChatSendResult
import com.ryderdavid.nacre.data.model.CreateChat
import com.ryderdavid.nacre.data.model.PermissionOutcome
import com.ryderdavid.nacre.data.model.SendChatMessage
import kotlinx.coroutines.flow.Flow

/**
 * What Chat needs from the paired computers; [com.ryderdavid.nacre.data.NacreRepository] in the
 * app. Chats are addressed by key, "<paired host>~<chat id>".
 */
interface ChatLink {
    /** Fetches every computer's chat list again and returns the merged list. */
    suspend fun refreshChats(): ChatListing
    suspend fun createChat(hostId: String, body: CreateChat): ChatRecord
    suspend fun chatDetail(key: String): BridgeClient.ChatDetail
    fun chatStream(key: String, after: Long): Flow<ChatStreamMessage>
    suspend fun sendChat(key: String, body: SendChatMessage): ChatSendResult
    suspend fun cancelChat(key: String)
    suspend fun deleteChat(key: String)
    suspend fun answerPermission(key: String, request: String, allow: Boolean, note: String?): PermissionOutcome
    /** The bytes of the stored image [image] a message of chat [key] sent, for its thumbnail. */
    suspend fun chatImage(key: String, image: String): ByteArray

    /**
     * A decision a chat's log carried (created or changed), tagged with its computer: kept with the
     * project's decisions, and announced when it newly waits. A no-op where nothing keeps them.
     */
    fun learnDecision(d: Decision) {}
}

/** The newest state of each decision [events] carry, tagged with the chat's computer [hostId]. */
fun latestDecisions(events: List<ChatEvent>, hostId: String): List<Decision> =
    events.filter { it.kind == ChatWire.DECISION }.mapNotNull { it.decision }
        .associateBy { it.id }.values.map { it.copy(hostId = hostId) }
