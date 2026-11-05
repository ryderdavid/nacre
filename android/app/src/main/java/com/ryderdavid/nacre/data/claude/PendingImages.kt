package com.ryderdavid.nacre.data.claude

import kotlin.math.max
import kotlin.math.roundToInt

/**
 * Whether a chat composer can attach images and send right now, given its draft
 * and the images pending with it. A reason is the sentence the disabled action
 * shows. Pending images are never dropped here: when the chat stops taking
 * images, Send waits until they are removed.
 */
data class ComposerGate(
    val canAttach: Boolean,
    val attachReason: String?,
    val canSend: Boolean,
    val sendReason: String?,
    /** How many more images this message can take. */
    val remaining: Int,
)

/** The reasons the composer gives for itself. */
object ComposerReasons {
    const val FULL = "A message can carry up to ${ImageBudget.PER_MESSAGE} images"
}

/**
 * Works out the composer's actions. [support] is whether the chat's target
 * takes images; [limit] is the chat's own image budget, asked for a message of
 * a given count and decoded size. [blankSends] lets an empty message send.
 */
fun composerGate(
    hasText: Boolean,
    pending: List<ImagePayload>,
    support: ImageSupport,
    limit: (count: Int, bytes: Long) -> String? = { _, _ -> null },
    ready: Boolean = true,
    busy: Boolean = false,
    blankSends: Boolean = false,
): ComposerGate {
    val bytes = pending.byteCount()
    val remaining = (ImageBudget.PER_MESSAGE - pending.size).coerceAtLeast(0)
    val attachReason = support.reason
        ?: limit(pending.size + 1, bytes + 1)
        ?: ComposerReasons.FULL.takeIf { remaining == 0 }
    val sendReason = if (pending.isEmpty()) null else support.reason ?: limit(pending.size, bytes)
    val idle = ready && !busy
    return ComposerGate(
        canAttach = idle && attachReason == null,
        attachReason = attachReason,
        canSend = idle && sendReason == null && (hasText || pending.isNotEmpty() || blankSends),
        sendReason = sendReason,
        remaining = remaining,
    )
}

/** [pending] with the image [id] removed. */
fun List<ImagePayload>.without(id: String) = filterNot { it.image.id == id }

/** [pending] with [picked] added, up to [ImageBudget.PER_MESSAGE]. */
fun List<ImagePayload>.plusPicked(picked: List<ImagePayload>) = (this + picked).take(ImageBudget.PER_MESSAGE)

/** Preparing a picked image: its type, its size and how it is re-encoded. Pure, so it can be tested off the device. */
object ImagePrep {
    const val MAX_EDGE = 1568
    val SUPPORTED = setOf("image/jpeg", "image/png", "image/gif", "image/webp")

    /** The image type [bytes] start with, or null when they aren't JPEG, PNG, GIF or WebP. */
    fun sniff(bytes: ByteArray): String? {
        fun at(i: Int) = if (i < bytes.size) bytes[i].toInt() and 0xff else -1
        fun ascii(from: Int, s: String) = s.indices.all { at(from + it) == s[it].code }
        return when {
            at(0) == 0xff && at(1) == 0xd8 && at(2) == 0xff -> "image/jpeg"
            at(0) == 0x89 && ascii(1, "PNG\r\n\u001a\n") -> "image/png"
            ascii(0, "GIF87a") || ascii(0, "GIF89a") -> "image/gif"
            ascii(0, "RIFF") && ascii(8, "WEBP") -> "image/webp"
            else -> null
        }
    }

    /** The size a [width] × [height] image is scaled to so its long edge is at most [maxEdge]. */
    fun fit(width: Int, height: Int, maxEdge: Int = MAX_EDGE): Pair<Int, Int> {
        val long = max(width, height)
        if (long <= maxEdge) return width to height
        val scale = maxEdge.toDouble() / long
        return (width * scale).roundToInt().coerceAtLeast(1) to (height * scale).roundToInt().coerceAtLeast(1)
    }

    /** The largest power-of-two subsample that still decodes at least [maxEdge] on the long edge. */
    fun sampleSize(width: Int, height: Int, maxEdge: Int = MAX_EDGE): Int {
        var sample = 1
        while (max(width, height) / (sample * 2) >= maxEdge) sample *= 2
        return sample
    }

    /** Whether the original bytes can be sent as they are. GIFs are always re-encoded, so only their first frame is sent. */
    fun keepsOriginal(mime: String, width: Int, height: Int, size: Int): Boolean =
        mime != "image/gif" && max(width, height) <= MAX_EDGE && size <= ImageBudget.IMAGE_BYTES

    /** The type a re-encoded image keeps: its own when Android can write it, PNG for a GIF's first frame. */
    fun encodeAs(mime: String) = if (mime == "image/gif") "image/png" else mime

    val JPEG_QUALITIES = listOf(90, 80, 70, 60, 50, 40)

    /**
     * Encodes a decoded image within [ImageBudget.IMAGE_BYTES]: first as [mime],
     * then as JPEG at falling quality. [encode] writes it in a type at a quality
     * and returns the bytes, or null when it can't. Null when nothing fits.
     */
    fun shrink(mime: String, encode: (mime: String, quality: Int) -> ByteArray?): Pair<String, ByteArray>? {
        val first = encodeAs(mime)
        encode(first, 90)?.takeIf { it.size <= ImageBudget.IMAGE_BYTES }?.let { return first to it }
        for (q in JPEG_QUALITIES) {
            encode("image/jpeg", q)?.takeIf { it.size <= ImageBudget.IMAGE_BYTES }?.let { return "image/jpeg" to it }
        }
        return null
    }

    /** [name] with the extension of [mime], when re-encoding changed its type. */
    fun rename(name: String, mime: String): String {
        val ext = ChatImage("", mime, name).extension
        val base = name.substringBeforeLast('.', name).ifBlank { "image" }
        val current = name.substringAfterLast('.', "").lowercase()
        val same = current == ext || (ext == "jpg" && current == "jpeg")
        return if (same) name else "$base.$ext"
    }
}

/** Why a picked file wasn't attached, as the snackbar says it. */
object SkipReasons {
    fun unsupported(name: String) = "$name isn't a JPEG, PNG, GIF or WebP image, so it wasn't attached"
    fun unreadable(name: String) = "Couldn't read $name, so it wasn't attached"
    fun tooLarge(name: String) = "$name couldn't be made smaller than 3 MB, so it wasn't attached"
}
