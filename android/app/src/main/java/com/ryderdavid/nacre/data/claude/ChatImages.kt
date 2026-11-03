package com.ryderdavid.nacre.data.claude

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.io.File
import java.io.IOException
import java.util.UUID

/**
 * An image attached to a chat message, by reference. The bytes live elsewhere:
 * for bead chats in a file under `chat-attachments/`, for the board assistant
 * in memory. Stored threads hold only this.
 */
@Serializable
data class ChatImage(
    val id: String,
    @SerialName("mime_type") val mimeType: String,
    @SerialName("file_name") val fileName: String,
) : java.io.Serializable {
    /** The file extension its saved copy uses. */
    val extension: String get() = when (mimeType) {
        "image/jpeg" -> "jpg"
        "image/png" -> "png"
        "image/gif" -> "gif"
        "image/webp" -> "webp"
        else -> "bin"
    }

    companion object {
        fun create(mimeType: String, fileName: String) = ChatImage(UUID.randomUUID().toString(), mimeType, fileName)
    }
}

/** An image with its bytes, ready to send. */
class ImagePayload(val image: ChatImage, val bytes: ByteArray) {
    override fun equals(other: Any?) = other is ImagePayload && other.image == image && other.bytes.contentEquals(bytes)
    override fun hashCode() = 31 * image.hashCode() + bytes.contentHashCode()
    override fun toString() = "ImagePayload(${image.fileName}, ${bytes.size} bytes)"
}

/** The limits images are held to, so a request never grows past what Claude accepts. */
object ImageBudget {
    const val PER_MESSAGE = 5

    /** Each image after downscaling, decoded. */
    const val IMAGE_BYTES = 3_000_000L

    /** Every image one request carries, counting earlier turns. */
    const val REQUEST_IMAGES = 20
    const val REQUEST_BYTES = 15_000_000L
}

/** What a prompt says about one image at its turn. */
sealed interface ImageMark {
    /** Sent with the request as image [number]. */
    data class Sent(val number: Int) : ImageMark

    /** Left out to keep the request within [ImageBudget]. */
    data object Omitted : ImageMark

    /** Its saved file is missing or can't be read. */
    data object Unavailable : ImageMark
}

/**
 * The images one request carries, as [images] numbered 1..N in that order
 * (oldest turn first, the current message last), and what each image of the
 * conversation becomes at its marker.
 */
data class RequestImages(val images: List<ImagePayload>, val marks: Map<String, ImageMark>) {
    companion object {
        val NONE = RequestImages(emptyList(), emptyMap())
    }
}

/**
 * Picks the images a request carries: [current]'s first, then earlier turns'
 * newest first, while the request stays within [ImageBudget.REQUEST_IMAGES]
 * and [ImageBudget.REQUEST_BYTES]. Only the turns a prompt shows
 * ([Prompts.HISTORY_TURNS]) are considered. [read] returns an image's bytes, or
 * null when its file is missing or unreadable.
 */
fun resolveImages(history: List<DraftTurn>, current: List<ChatImage>, read: (ChatImage) -> ByteArray?): RequestImages {
    val shown = history.takeLast(Prompts.HISTORY_TURNS)
    if (current.isEmpty() && shown.none { it.images.isNotEmpty() }) return RequestImages.NONE
    val chosen = mutableMapOf<String, ByteArray>()
    val marks = mutableMapOf<String, ImageMark>()
    var bytes = 0L
    val newestFirst = current + shown.asReversed().flatMap { it.images }
    for (image in newestFirst) {
        if (image.id in marks || image.id in chosen) continue
        if (chosen.size >= ImageBudget.REQUEST_IMAGES) { marks[image.id] = ImageMark.Omitted; continue }
        val data = read(image)
        when {
            data == null -> marks[image.id] = ImageMark.Unavailable
            bytes + data.size > ImageBudget.REQUEST_BYTES -> marks[image.id] = ImageMark.Omitted
            else -> { chosen[image.id] = data; bytes += data.size }
        }
    }
    val images = mutableListOf<ImagePayload>()
    for (image in shown.flatMap { it.images } + current) {
        val data = chosen.remove(image.id) ?: continue
        images += ImagePayload(image, data)
        marks[image.id] = ImageMark.Sent(images.size)
    }
    return RequestImages(images, marks)
}

/** [text] with a marker for each of its [images]; just [text] when there are none. */
internal fun withMarkers(text: String, images: List<ChatImage>, marks: Map<String, ImageMark>): String {
    if (images.isEmpty()) return text
    val markers = images.joinToString(" ") { image ->
        when (val m = marks[image.id]) {
            is ImageMark.Sent -> "[image ${m.number}: ${image.fileName}]"
            ImageMark.Unavailable -> "[image unavailable: ${image.fileName}]"
            ImageMark.Omitted, null -> "[image omitted: ${image.fileName}]"
        }
    }
    return if (text.isBlank()) markers else "$text\n$markers"
}

/** Raised when an attachment couldn't be saved; nothing was sent and nothing of the message was kept. */
class ChatAttachmentException(message: String, cause: Throwable? = null) : IOException(message, cause)

/**
 * Bead chat attachment files, one per image as `<id>.<ext>` in [dir]. Each is
 * written to a temporary file first and renamed into place, so a file under
 * its final name is always complete.
 */
class ChatAttachments(
    private val dir: File,
    private val writeBytes: (File, ByteArray) -> Unit = { f, b -> f.writeBytes(b) },
) {
    fun file(image: ChatImage) = File(dir, "${image.id}.${image.extension}")

    /** Saves every one of [images], or none of them. */
    fun save(images: List<ImagePayload>) {
        if (images.isEmpty()) return
        val written = mutableListOf<File>()
        try {
            if (!dir.isDirectory && !dir.mkdirs()) throw IOException("couldn't create ${dir.name}")
            for (p in images) {
                val target = file(p.image)
                val tmp = File(dir, "${target.name}$TMP")
                written += tmp
                writeBytes(tmp, p.bytes)
                if (!tmp.renameTo(target)) throw IOException("couldn't save ${p.image.fileName}")
                written += target
            }
        } catch (e: Exception) {
            written.forEach { it.delete() }
            throw ChatAttachmentException("Couldn't save the attached images: ${e.message ?: e::class.simpleName}", e)
        }
    }

    /** The image's bytes, or null when its file is missing or unreadable. */
    fun read(image: ChatImage): ByteArray? = runCatching { file(image).readBytes() }.getOrNull()

    fun delete(ids: Collection<String>) {
        if (ids.isEmpty()) return
        val gone = ids.toSet()
        dir.listFiles()?.forEach { if (idOf(it) in gone) it.delete() }
    }

    /** Deletes every file no thread refers to, and any temporary file left by an interrupted save. */
    fun sweep(referenced: Set<String>) {
        dir.listFiles()?.forEach { if (it.name.endsWith(TMP) || idOf(it) !in referenced) it.delete() }
    }

    private fun idOf(f: File) = f.name.substringBefore('.')

    private companion object {
        const val TMP = ".tmp"
    }
}
