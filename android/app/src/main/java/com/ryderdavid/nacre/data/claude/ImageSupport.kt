package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.model.AssistImage
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

/** An image as nacred's `images` entry: media type, file name and base64 bytes. */
internal fun ImagePayload.toAssist() = AssistImage(image.mimeType, image.fileName, Base64.getEncoder().encodeToString(bytes))

/** The decoded bytes of [images], as the budgets count them. */
internal fun List<ImagePayload>.byteCount(): Long = sumOf { it.bytes.size.toLong() }
