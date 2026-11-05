package com.ryderdavid.nacre.data.claude

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.util.UUID

/**
 * An image attached to a Chat message, by reference; its bytes travel beside
 * it in [ImagePayload] until the message is sent.
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

    /** Every image one chat may send, counting earlier messages. */
    const val REQUEST_IMAGES = 20
    const val REQUEST_BYTES = 15_000_000L
}
