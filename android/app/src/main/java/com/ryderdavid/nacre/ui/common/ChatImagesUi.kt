package com.ryderdavid.nacre.ui.common

import android.content.ContentResolver
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.Canvas
import android.graphics.ImageDecoder
import android.net.Uri
import android.provider.OpenableColumns
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.AddPhotoAlternate
import androidx.compose.material.icons.rounded.BrokenImage
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.claude.ChatImage
import com.ryderdavid.nacre.data.claude.ComposerGate
import com.ryderdavid.nacre.data.claude.ImageBudget
import com.ryderdavid.nacre.data.claude.ImagePayload
import com.ryderdavid.nacre.data.claude.ImagePrep
import com.ryderdavid.nacre.data.claude.SkipReasons
import com.ryderdavid.nacre.data.claude.plusPicked
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.ByteArrayOutputStream
import java.nio.ByteBuffer

/** A picked image after preparing it: ready to attach, or skipped with the reason the snackbar gives. */
sealed interface PreparedImage {
    data class Ready(val payload: ImagePayload) : PreparedImage
    data class Skipped(val reason: String) : PreparedImage
}

/** Files larger than this aren't read at all; nothing that size could be brought under 3 MB sensibly. */
private const val MAX_SOURCE_BYTES = 64L * 1024 * 1024

/**
 * Reads the picked image at [uri] and prepares it to send: JPEG, PNG, GIF or
 * WebP only, its long edge at most 1568 px and at most 3 MB, re-encoded (as
 * JPEG at falling quality when needed) only when it has to be. A GIF sends its
 * first frame. Runs off the main thread.
 */
fun prepareImage(resolver: ContentResolver, uri: Uri): PreparedImage {
    val name = displayName(resolver, uri)
    val bytes = runCatching {
        resolver.openInputStream(uri)?.use { input ->
            val out = ByteArrayOutputStream()
            val buf = ByteArray(64 * 1024)
            var total = 0L
            while (true) {
                val n = input.read(buf)
                if (n < 0) break
                total += n
                if (total > MAX_SOURCE_BYTES) return PreparedImage.Skipped(SkipReasons.tooLarge(name))
                out.write(buf, 0, n)
            }
            out.toByteArray()
        }
    }.getOrNull() ?: return PreparedImage.Skipped(SkipReasons.unreadable(name))
    val mime = ImagePrep.sniff(bytes) ?: return PreparedImage.Skipped(SkipReasons.unsupported(name))

    val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
    BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
    if (bounds.outWidth <= 0 || bounds.outHeight <= 0) return PreparedImage.Skipped(SkipReasons.unreadable(name))
    if (ImagePrep.keepsOriginal(mime, bounds.outWidth, bounds.outHeight, bytes.size)) {
        return PreparedImage.Ready(ImagePayload(ChatImage.create(mime, name), bytes))
    }

    val bitmap = runCatching {
        ImageDecoder.decodeBitmap(ImageDecoder.createSource(ByteBuffer.wrap(bytes))) { decoder, info, _ ->
            val (w, h) = ImagePrep.fit(info.size.width, info.size.height)
            decoder.setTargetSize(w, h)
            decoder.allocator = ImageDecoder.ALLOCATOR_SOFTWARE
        }
    }.getOrNull() ?: return PreparedImage.Skipped(SkipReasons.unreadable(name))
    val shrunk = try {
        ImagePrep.shrink(mime) { type, quality -> encode(bitmap, type, quality) }
    } finally {
        bitmap.recycle()
    }
    val (type, data) = shrunk ?: return PreparedImage.Skipped(SkipReasons.tooLarge(name))
    return PreparedImage.Ready(ImagePayload(ChatImage.create(type, ImagePrep.rename(name, type)), data))
}

private fun encode(bitmap: Bitmap, mime: String, quality: Int): ByteArray? = runCatching {
    val format = when (mime) {
        "image/png" -> Bitmap.CompressFormat.PNG
        "image/webp" -> Bitmap.CompressFormat.WEBP_LOSSY
        else -> Bitmap.CompressFormat.JPEG
    }
    // JPEG has no transparency: put a transparent image on white rather than black.
    val source = if (format == Bitmap.CompressFormat.JPEG && bitmap.hasAlpha()) {
        Bitmap.createBitmap(bitmap.width, bitmap.height, Bitmap.Config.ARGB_8888).also {
            Canvas(it).apply { drawColor(android.graphics.Color.WHITE); drawBitmap(bitmap, 0f, 0f, null) }
        }
    } else bitmap
    try {
        ByteArrayOutputStream().also { if (!source.compress(format, quality, it)) error("couldn't encode") }.toByteArray()
    } finally {
        if (source !== bitmap) source.recycle()
    }
}.getOrNull()

private fun displayName(resolver: ContentResolver, uri: Uri): String = runCatching {
    resolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { c ->
        if (c.moveToFirst()) c.getString(0) else null
    }
}.getOrNull()?.ifBlank { null } ?: uri.lastPathSegment?.substringAfterLast('/')?.ifBlank { null } ?: "image"

/**
 * Opens the system photo picker (`PickMultipleVisualMedia(ImageOnly)`) for the
 * message whose composer state is kept under [viewKey] in the chat stack, up to
 * [remaining] images. What was picked is prepared off the main thread (a long
 * edge of at most 1568 px and at most 3 MB, re-encoded when needed); the ready
 * images join the pending ones and any skipped file is named in a snackbar.
 */
@Composable
fun rememberImageAttacher(viewKey: String, remaining: Int): () -> Unit {
    val container = LocalContainer.current
    val resolver = LocalContext.current.contentResolver
    val scope = rememberCoroutineScope()
    val key by rememberUpdatedState(viewKey)
    val onPicked: (List<Uri>) -> Unit = { uris ->
        if (uris.isNotEmpty()) scope.launch {
            val prepared = withContext(Dispatchers.IO) { uris.map { prepareImage(resolver, it) } }
            val ready = prepared.filterIsInstance<PreparedImage.Ready>().map { it.payload }
            if (ready.isNotEmpty()) container.chatStack.updateView(key) { it.copy(images = it.images.plusPicked(ready)) }
            prepared.filterIsInstance<PreparedImage.Skipped>().takeIf { it.isNotEmpty() }?.let { skipped ->
                container.repository.notify(skipped.joinToString("\n") { it.reason })
            }
        }
    }
    // The multiple picker needs room for at least two; a single last slot uses the single one.
    val many = rememberLauncherForActivityResult(
        ActivityResultContracts.PickMultipleVisualMedia(remaining.coerceIn(2, ImageBudget.PER_MESSAGE)),
    ) { onPicked(it.take(remaining)) }
    val one = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri -> onPicked(listOfNotNull(uri)) }
    val request = PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)
    return {
        when {
            remaining >= 2 -> many.launch(request)
            remaining == 1 -> one.launch(request)
        }
    }
}

/** The Attach image action; disabled with its reason when the chat can't take another image. */
@Composable
fun AttachImageButton(gate: ComposerGate, onClick: () -> Unit) {
    IconButton(
        onClick = onClick, enabled = gate.canAttach,
        modifier = Modifier.semantics { contentDescription = gate.attachReason?.let { "Attach image. $it" } ?: "Attach image" },
    ) { Icon(Icons.Rounded.AddPhotoAlternate, null) }
}

/** The reason Send or Attach image is disabled, under the composer; nothing when both are available. */
@Composable
fun ComposerReason(gate: ComposerGate, modifier: Modifier = Modifier) {
    val reason = gate.sendReason ?: gate.attachReason?.takeIf { gate.remaining > 0 } ?: return
    Text(
        reason, modifier = modifier,
        style = MaterialTheme.typography.labelSmall,
        color = if (gate.sendReason != null) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

/** The images waiting to go with the next message, each removable. */
@Composable
fun PendingImageRow(images: List<ImagePayload>, onRemove: (String) -> Unit, modifier: Modifier = Modifier) {
    if (images.isEmpty()) return
    LazyRow(modifier, horizontalArrangement = Arrangement.spacedBy(8.dp), contentPadding = PaddingValues(horizontal = 8.dp)) {
        items(images, key = { it.image.id }) { p ->
            Box {
                ImageThumb(p.image, { p.bytes }, 64.dp)
                Surface(
                    shape = CircleShape, color = MaterialTheme.colorScheme.surfaceContainerHighest,
                    modifier = Modifier.align(Alignment.TopEnd).padding(2.dp).size(24.dp),
                ) {
                    IconButton(onClick = { onRemove(p.image.id) }, modifier = Modifier.size(24.dp)) {
                        Icon(Icons.Rounded.Close, "Remove ${p.image.fileName}", Modifier.size(16.dp))
                    }
                }
            }
        }
    }
}

/** One thumbnail, decoded small off the main thread; "Image unavailable" when its bytes are missing or unreadable. */
@Composable
fun ImageThumb(image: ChatImage, bytes: suspend () -> ByteArray?, size: Dp) {
    val px = with(androidx.compose.ui.platform.LocalDensity.current) { size.roundToPx() }
    val bitmap by produceState<Result<ImageBitmap>?>(null, image.id) {
        value = withContext(Dispatchers.IO) {
            runCatching { decodeThumb(bytes() ?: error("missing"), px) ?: error("undecodable") }
        }
    }
    val shape = RoundedCornerShape(10.dp)
    Surface(shape = shape, color = MaterialTheme.colorScheme.surfaceContainerHigh, modifier = Modifier.size(size)) {
        val b = bitmap
        when {
            b == null -> Unit
            b.isSuccess -> Image(b.getOrThrow(), image.fileName, Modifier.size(size).clip(shape), contentScale = ContentScale.Crop)
            else -> Box(Modifier.padding(4.dp), contentAlignment = Alignment.Center) {
                androidx.compose.foundation.layout.Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    Icon(Icons.Rounded.BrokenImage, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
                    Text(
                        com.ryderdavid.nacre.data.chat.IMAGE_UNAVAILABLE, style = MaterialTheme.typography.labelSmall, textAlign = TextAlign.Center,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

private fun decodeThumb(bytes: ByteArray, px: Int): ImageBitmap? {
    val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
    BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
    if (bounds.outWidth <= 0) return null
    val opts = BitmapFactory.Options().apply { inSampleSize = ImagePrep.sampleSize(bounds.outWidth, bounds.outHeight, px.coerceAtLeast(1)) }
    return BitmapFactory.decodeByteArray(bytes, 0, bytes.size, opts)?.asImageBitmap()
}

/**
 * A sent Chat message's images, fetched from nacred (`GET /v1/chats/{id}/images/{image}`) for
 * thumbnails; one that can't be fetched shows "Image unavailable".
 */
@Composable
fun ChatSentImages(chatKey: String, images: List<com.ryderdavid.nacre.data.model.ChatImageRef>, modifier: Modifier = Modifier) {
    if (images.isEmpty()) return
    val thumbs = LocalContainer.current.chats.thumbs
    androidx.compose.foundation.layout.FlowRow(
        modifier, horizontalArrangement = Arrangement.spacedBy(6.dp, Alignment.End), verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        images.forEach { ref ->
            ImageThumb(ChatImage(ref.id, ref.mediaType, ref.name.ifBlank { ref.id }), { thumbs.bytes(chatKey, ref) }, 96.dp)
        }
    }
}
