package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.Connection
import com.ryderdavid.nacre.data.HostState
import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.bridge.ChatStreamMessage
import com.ryderdavid.nacre.data.model.ChatEvent
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.ChatSendResult
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.CreateChat
import com.ryderdavid.nacre.data.model.Features
import com.ryderdavid.nacre.data.model.Health
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.PermissionOutcome
import com.ryderdavid.nacre.data.model.SendChatMessage
import com.ryderdavid.nacre.data.settings.Host
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.withTimeout
import java.io.IOException

internal fun host(id: String, name: String, vararg features: String, connection: Connection? = null) =
    HostState(Host(id, "hid-$id", name, "http://$id", "t"), connection ?: Connection.Online(Health(hostName = name, features = features.toList())))

internal val CHAT = arrayOf(Features.CLAUDE, Features.AGENT_CHAT)

internal fun ev(seq: Long, kind: String, text: String = "", req: String = "", allowed: Boolean? = null, by: String = "", reason: String = "", tool: String = "") =
    ChatEvent(seq = seq, kind = kind, text = text, requestId = req, allowed = allowed, by = by, reason = reason, tool = tool)

internal fun permission(seq: Long, req: String, step: String = "Bash · ./gradlew assembleDebug") =
    ev(seq, ChatWire.PERMISSION, text = step, req = req, reason = "Builds touch files outside the project", tool = "Bash")

/** A stream that plays [msgs] and then drops, or ends cleanly when [drop] is false. */
internal fun stream(vararg msgs: ChatStreamMessage, drop: Boolean = false): Flow<ChatStreamMessage> = flow {
    msgs.forEach { emit(it) }
    if (drop) throw IOException("connection reset")
}

internal fun events(vararg e: ChatEvent) = e.map { ChatStreamMessage.Event(it) }.toTypedArray()

internal suspend fun until(timeoutMs: Long = 5_000, cond: () -> Boolean) {
    withTimeout(timeoutMs) { while (!cond()) delay(5) }
}

/** A computer with chats that plays back scripted streams. */
internal class FakeChatLink(
    var listing: ChatListing = ChatListing(),
    var detail: (String) -> BridgeClient.ChatDetail = { key -> BridgeClient.ChatDetail(ChatRecord(Keys.id(key), state = ChatWire.IDLE)) },
) : ChatLink {
    val streams = mutableListOf<(String, Long) -> Flow<ChatStreamMessage>>()
    val opened = mutableListOf<Pair<String, Long>>()
    val sent = mutableListOf<SendChatMessage>()
    var sendFailures = 0
    var answers = mutableListOf<Triple<String, Boolean, String?>>()
    var answer: (String, Boolean, String?) -> PermissionOutcome = { req, allow, note ->
        PermissionOutcome(ev(0, ChatWire.PERMISSION_ANSWER, req = req, allowed = allow, by = "phone", reason = if (allow) "" else note.orEmpty()), conflict = false)
    }
    var cancelled = 0
    /** Stored images by id; a missing one fails like a 404. */
    val images = mutableMapOf<String, ByteArray>()
    var imageFetches = 0
    /** Thrown instead of answering a send, e.g. nacred's 400, 409 or 413. */
    var refuse: Exception? = null

    override suspend fun refreshChats() = listing
    /** The draft events "Where does this belong?" raised intake from, and what each came to. */
    val draftRaises = mutableListOf<Pair<String, Long>>()
    var draftRaise: (String, Long) -> DraftIntakeResult = { _, _ -> DraftIntakeResult.Raised(com.ryderdavid.nacre.data.model.Decision(id = "dc_1", kind = "intake")) }
    override suspend fun raiseDraftIntake(key: String, seq: Long): DraftIntakeResult {
        draftRaises += key to seq
        return draftRaise(key, seq)
    }
    /** Decisions the chats' logs carried, in the order they were handed on. */
    val learned = mutableListOf<com.ryderdavid.nacre.data.model.Decision>()
    override fun learnDecision(d: com.ryderdavid.nacre.data.model.Decision) { synchronized(learned) { learned += d } }
    val created = mutableListOf<Pair<String, CreateChat>>()
    override suspend fun createChat(hostId: String, body: CreateChat): ChatRecord {
        created += hostId to body
        return ChatRecord("new", repo = body.repo, newEffort = body.newEffort, hostId = hostId)
    }
    override suspend fun chatDetail(key: String) = detail(key)
    override fun chatStream(key: String, after: Long): Flow<ChatStreamMessage> {
        opened += key to after
        return streams.removeAt(0)(key, after)
    }
    override suspend fun sendChat(key: String, body: SendChatMessage): ChatSendResult {
        sent += body
        refuse?.let { throw it }
        if (sendFailures > 0) {
            sendFailures--
            throw IOException("response lost")
        }
        return ChatSendResult(body.turnId, started = sent.count { it.turnId == body.turnId } == 1, chat = ChatRecord(Keys.id(key), state = ChatWire.RUNNING))
    }
    override suspend fun cancelChat(key: String) { cancelled++ }
    override suspend fun deleteChat(key: String) = Unit
    override suspend fun answerPermission(key: String, request: String, allow: Boolean, note: String?): PermissionOutcome {
        answers += Triple(request, allow, note)
        return answer(request, allow, note)
    }
    override suspend fun chatImage(key: String, image: String): ByteArray {
        imageFetches++
        return images[image] ?: throw com.ryderdavid.nacre.data.bridge.BridgeException("unknown image", code = 404)
    }
}

internal class RecordingNotices : ChatNotices {
    val posted = mutableListOf<String>()
    val cleared = mutableListOf<String>()
    override fun post(chat: ChatRecord, card: PermissionCard) { synchronized(this) { posted += chat.key + "/" + card.requestId } }
    override fun clear(chatKey: String, requestId: String) { synchronized(this) { cleared += "$chatKey/$requestId" } }
}

internal fun quickBackoff() = Backoff(pause = {})
