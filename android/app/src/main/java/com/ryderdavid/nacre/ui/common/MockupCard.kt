package com.ryderdavid.nacre.ui.common

import android.annotation.SuppressLint
import android.content.Context
import android.graphics.BitmapFactory
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.rememberTransformableState
import androidx.compose.foundation.gestures.transformable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.Fullscreen
import androidx.compose.material.icons.rounded.Image
import androidx.compose.material.icons.rounded.Web
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.ryderdavid.nacre.data.chat.MockupRef
import com.ryderdavid.nacre.data.chat.Mockups

private sealed interface MockupState {
    data object Loading : MockupState
    data class Shown(val bytes: ByteArray) : MockupState
    data class Failed(val message: String) : MockupState
}

/**
 * A mockup the agent showed, inline in the chat: an HTML page in a sandboxed web view (scripts run; no
 * navigation, no file or content access, a strict CSP) or an image, with a full screen that pinch-zooms.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun MockupCard(chatKey: String, ref: MockupRef, modifier: Modifier = Modifier) {
    val cache = LocalContainer.current.chats.mockups
    var attempt by remember(chatKey, ref) { mutableIntStateOf(0) }
    var state by remember(chatKey, ref) { mutableStateOf<MockupState>(MockupState.Loading) }
    var full by remember(chatKey, ref) { mutableStateOf(false) }
    LaunchedEffect(chatKey, ref, attempt) {
        state = MockupState.Loading
        state = cache.bytes(chatKey, ref).fold({ MockupState.Shown(it) }, { MockupState.Failed(it.message ?: "unavailable") })
    }
    val web = ref.isHtml || ref.isSvg
    Surface(shape = RoundedCornerShape(24.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh, modifier = modifier.fillMaxWidth()) {
        Column {
            Row(Modifier.padding(start = 16.dp, end = 8.dp, top = 6.dp, bottom = 6.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Icon(if (web) Icons.Rounded.Web else Icons.Rounded.Image, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                Column(Modifier.weight(1f)) {
                    Text("Mockup", style = MaterialTheme.typography.labelLarge)
                    Text(ref.name, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
                FilledTonalIconButton(onClick = { full = true }, enabled = state is MockupState.Shown, modifier = Modifier.size(48.dp)) {
                    Icon(Icons.Rounded.Fullscreen, "Open ${ref.name} full screen")
                }
            }
            Box(
                Modifier.fillMaxWidth().padding(start = 8.dp, end = 8.dp, bottom = 8.dp).clip(RoundedCornerShape(18.dp))
                    .background(MaterialTheme.colorScheme.surfaceContainerLowest),
                contentAlignment = Alignment.Center,
            ) {
                when (val s = state) {
                    MockupState.Loading -> Box(Modifier.fillMaxWidth().height(200.dp), contentAlignment = Alignment.Center) { LoadingIndicator() }
                    is MockupState.Failed -> Column(Modifier.fillMaxWidth().padding(16.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                        Text("Couldn't load ${ref.name}: ${s.message}", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        TextButton(onClick = { attempt++ }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Try again") }
                    }
                    is MockupState.Shown -> if (web) {
                        MockupWebView(Mockups.page(ref, s.bytes), zoom = false, modifier = Modifier.fillMaxWidth().height(if (ref.isSvg) 280.dp else 420.dp))
                    } else {
                        MockupImage(s.bytes, ref.name, Modifier.fillMaxWidth().heightIn(max = 520.dp).clickable { full = true })
                    }
                }
            }
        }
    }
    val shown = state as? MockupState.Shown
    if (full && shown != null) MockupFullScreen(ref, shown.bytes) { full = false }
}

@Composable
private fun MockupFullScreen(ref: MockupRef, bytes: ByteArray, onClose: () -> Unit) {
    Dialog(onDismissRequest = onClose, properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false)) {
        Surface(color = MaterialTheme.colorScheme.surface, modifier = Modifier.fillMaxSize()) {
            Column(Modifier.statusBarsPadding().navigationBarsPadding()) {
                Row(Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                    IconButton(onClick = onClose, modifier = Modifier.size(48.dp)) { Icon(Icons.Rounded.Close, "Close") }
                    Text(ref.name, style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                }
                if (ref.isHtml || ref.isSvg) MockupWebView(Mockups.page(ref, bytes), zoom = true, modifier = Modifier.fillMaxSize())
                else ZoomableImage(bytes, ref.name, Modifier.fillMaxSize())
            }
        }
    }
}

@Composable
private fun MockupImage(bytes: ByteArray, name: String, modifier: Modifier) {
    val bitmap = remember(bytes) { BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap() }
    if (bitmap == null) Text("$name isn't an image this phone can show", style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(16.dp))
    else Image(bitmap, name, modifier, contentScale = ContentScale.FillWidth)
}

@Composable
private fun ZoomableImage(bytes: ByteArray, name: String, modifier: Modifier) {
    var scale by remember { mutableFloatStateOf(1f) }
    var offset by remember { mutableStateOf(Offset.Zero) }
    val transform = rememberTransformableState { zoom, pan, _ ->
        scale = (scale * zoom).coerceIn(1f, 8f)
        offset = if (scale == 1f) Offset.Zero else offset + pan
    }
    Box(modifier.transformable(transform).semantics { contentDescription = "$name, pinch to zoom" }, contentAlignment = Alignment.Center) {
        MockupImage(bytes, name, Modifier.fillMaxWidth().graphicsLayer(scaleX = scale, scaleY = scale, translationX = offset.x, translationY = offset.y))
    }
}

/** A web view that runs the page's scripts and nothing else: no navigation, no file or content access, no storage. */
@SuppressLint("SetJavaScriptEnabled")
private fun sandboxedWebView(context: Context, zoom: Boolean) = WebView(context).apply {
    settings.javaScriptEnabled = true
    settings.allowFileAccess = false
    settings.allowContentAccess = false
    settings.domStorageEnabled = false
    settings.javaScriptCanOpenWindowsAutomatically = false
    settings.setSupportMultipleWindows(false)
    settings.setGeolocationEnabled(false)
    settings.setSupportZoom(zoom)
    settings.builtInZoomControls = zoom
    settings.displayZoomControls = false
    settings.useWideViewPort = zoom
    settings.loadWithOverviewMode = zoom
    webViewClient = object : WebViewClient() {
        override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest) = true
    }
}

@Composable
private fun MockupWebView(page: String, zoom: Boolean, modifier: Modifier) {
    AndroidView(
        factory = { ctx -> sandboxedWebView(ctx, zoom).also { it.tag = page; it.loadDataWithBaseURL(null, page, "text/html", "utf-8", null) } },
        update = { v -> if (v.tag != page) { v.tag = page; v.loadDataWithBaseURL(null, page, "text/html", "utf-8", null) } },
        onRelease = { it.destroy() },
        modifier = modifier,
    )
}
