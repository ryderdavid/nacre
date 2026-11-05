package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.claude.ComposerGate
import com.ryderdavid.nacre.data.claude.ImagePayload
import com.ryderdavid.nacre.data.claude.ImageSupport
import com.ryderdavid.nacre.data.claude.composerGate
import com.ryderdavid.nacre.data.model.ChatImageRef
import com.ryderdavid.nacre.data.model.ChatRecord
import kotlinx.coroutines.CancellationException

/**
 * What one chat may send in images over its life, as nacred holds it too: the resumed session
 * keeps earlier images, so only the current message's are sent, and the chat as a whole is bounded.
 */
object ChatImageLimits {
    const val PER_CHAT = 20
    const val PER_CHAT_BYTES = 15_000_000L
    const val REASON = "This chat has reached its image limit. Start a new chat to send more images."
}

/** What a thumbnail shows when its image can't be fetched or read. */
const val IMAGE_UNAVAILABLE = "Image unavailable"

/**
 * The image-limit reason when a message of [count] images and [bytes] decoded bytes would take
 * [chat] past [ChatImageLimits.PER_CHAT] images or [ChatImageLimits.PER_CHAT_BYTES]; null when it
 * fits, or carries no images.
 */
fun chatImageLimit(chat: ChatRecord?, count: Int, bytes: Long): String? {
    if (count <= 0) return null
    val sentCount = chat?.imageCount ?: 0
    val sentBytes = chat?.imageBytes ?: 0L
    val over = sentCount + count > ChatImageLimits.PER_CHAT || sentBytes + bytes > ChatImageLimits.PER_CHAT_BYTES
    return if (over) ChatImageLimits.REASON else null
}

/**
 * Chat's composer actions: Attach image while the message has room for another image (5 per
 * message) and the chat's own budget allows one more; Send with text, images or both, unless the
 * pending images would pass the chat's budget, in which case they stay pending and Send gives the
 * image-limit reason. [canSendNow] is false while a turn runs or the chat is at its runaway limit.
 */
fun chatComposerGate(draft: String, pending: List<ImagePayload>, chat: ChatRecord?, canSendNow: Boolean): ComposerGate {
    val g = composerGate(
        hasText = draft.isNotBlank(),
        pending = pending,
        support = ImageSupport.YES,
        limit = { count, bytes -> chatImageLimit(chat, count, bytes) },
    )
    return g.copy(canSend = g.canSend && canSendNow)
}

/** What a sent image's thumbnail shows. */
sealed interface ThumbState {
    data object Loading : ThumbState
    class Shown(val bytes: ByteArray) : ThumbState
    data object Unavailable : ThumbState
}

/** Fetches a thumbnail's bytes; any failure, or nothing fetched, is [ThumbState.Unavailable]. */
suspend fun loadThumb(fetch: suspend () -> ByteArray?): ThumbState = try {
    fetch()?.takeIf { it.isNotEmpty() }?.let { ThumbState.Shown(it) } ?: ThumbState.Unavailable
} catch (e: CancellationException) {
    throw e
} catch (_: Exception) {
    ThumbState.Unavailable
}

/**
 * Sent images fetched from nacred for thumbnails, the most recent [max] kept in memory so
 * scrolling and reopening a chat don't fetch them again. A failed fetch is null and isn't kept, so
 * it is tried again the next time the thumbnail is shown.
 */
class ChatThumbs(private val link: ChatLink, private val max: Int = 40) {
    private val cache = object : LinkedHashMap<String, ByteArray>(16, 0.75f, true) {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<String, ByteArray>?) = size > max
    }

    suspend fun bytes(chatKey: String, image: ChatImageRef): ByteArray? {
        val id = "$chatKey/${image.id}"
        synchronized(cache) { cache[id] }?.let { return it }
        val fetched = try {
            link.chatImage(chatKey, image.id)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            null
        } ?: return null
        synchronized(cache) { cache[id] = fetched }
        return fetched
    }
}
