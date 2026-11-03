package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.Connection
import com.ryderdavid.nacre.data.HostState
import com.ryderdavid.nacre.data.model.AssistImage
import com.ryderdavid.nacre.data.model.Features
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.settings.AppSettings
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import java.util.Base64

/**
 * Whether a chat can take images right now. When it can't, [reason] is the
 * sentence the disabled Attach image and Send show; pending images are kept.
 */
data class ImageSupport(val reason: String?) {
    val ok get() = reason == null

    companion object {
        val YES = ImageSupport(null)
    }
}

/** The reasons image sends are refused, worded for the person holding the phone. */
object ImageReasons {
    fun update(computer: String) = "Update nacred on $computer to send images"

    fun unreachable(computer: String) = "Can't reach $computer to check that it can take images"

    const val ACROSS_BOARDS = "Images can't be sent to the assistant across boards"

    const val CONVERSATION_LIMIT = "This conversation has reached its image limit. Clear the assistant to send more images."
}

/**
 * Why the Mac computer [computer] can't take images, or null when it can: its
 * reported [features] must list `chat_images`. Null [features] means it isn't
 * connected, so what its nacred can do isn't known.
 */
internal fun macImageBlocker(computer: String, features: List<String>?): String? = when {
    features == null -> ImageReasons.unreachable(computer)
    Features.CHAT_IMAGES in features -> null
    else -> ImageReasons.update(computer)
}

/**
 * Whether a chat about [repoKey] can take images: on the API backend always,
 * on the Mac backend only when the computer in the repository key (the one the
 * turn is sent to) lists `chat_images`. A null [repoKey] is the assistant
 * across boards, which takes no images on either backend.
 */
fun imageSupport(onMac: Boolean, repoKey: String?, hosts: List<HostState>): ImageSupport {
    if (repoKey == null) return ImageSupport(ImageReasons.ACROSS_BOARDS)
    if (!onMac) return ImageSupport.YES
    val host = hosts.firstOrNull { it.host.id == Keys.host(repoKey) }
    val name = host?.host?.name?.ifBlank { null } ?: "this computer"
    val features = (host?.connection as? Connection.Online)?.health?.features
    return ImageSupport(macImageBlocker(name, features))
}

/**
 * [imageSupport] kept current: it is worked out again whenever a computer's
 * features refresh and whenever the backend setting changes. The backends
 * check again at send.
 */
class ChatImageSupport(private val settings: Flow<AppSettings>, private val hosts: Flow<List<HostState>>) {
    fun of(repoKey: String?): Flow<ImageSupport> =
        combine(settings.map { it.onMac }.distinctUntilChanged(), hosts) { onMac, h -> imageSupport(onMac, repoKey, h) }
            .distinctUntilChanged()
}

/** An image as nacred's `images` entry: media type, file name and base64 bytes. */
internal fun ImagePayload.toAssist() = AssistImage(image.mimeType, image.fileName, Base64.getEncoder().encodeToString(bytes))

/** The decoded bytes of [images], as the budgets count them. */
internal fun List<ImagePayload>.byteCount(): Long = sumOf { it.bytes.size.toLong() }
