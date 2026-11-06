package com.ryderdavid.nacre.ui.workspace.spec

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.contextmenu.builder.item
import androidx.compose.foundation.text.contextmenu.modifier.appendTextContextMenuComponents
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Chat
import androidx.compose.material.icons.automirrored.rounded.FormatListBulleted
import androidx.compose.material.icons.rounded.KeyboardArrowDown
import androidx.compose.material.icons.rounded.KeyboardArrowUp
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material.icons.rounded.WarningAmber
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FloatingToolbarDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.HorizontalFloatingToolbar
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.isShiftPressed
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.ui.common.ErrorState
import com.ryderdavid.nacre.ui.common.LoadingState
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.MessageState
import com.ryderdavid.nacre.ui.workspace.ChatReference
import com.ryderdavid.nacre.ui.workspace.LocalAskAbout
import com.ryderdavid.nacre.ui.workspace.LocalViewing
import kotlinx.coroutines.launch

/**
 * A change's document reader: the selector, outline, find, next and previous change, selectable text,
 * labelled requirement deltas, and Ask about the selection, with a vibrant floating toolbar and FAB.
 */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalMaterial3ExpressiveApi::class)
@Composable
internal fun DocumentReaderPane(vm: SpecPanelViewModel, screen: SpecScreen.Document, modifier: Modifier) {
    val repo = LocalContainer.current.repository
    val board by repo.board(vm.repoKey).collectAsState()
    val envelopes by vm.envelopes.collectAsState()
    val summary = board.data?.changes?.changes?.firstOrNull { it.id == screen.changeId }
    val loaded = envelopes[screen.changeId]
    val change = loaded?.data?.change
    LaunchedEffect(summary?.updatedAt) { summary?.let { vm.load(it) } }
    val name = change?.changeName?.ifBlank { null } ?: summary?.name ?: screen.changeId
    val doc = remember(change?.artifacts, screen.doc, name) { change?.let { ReaderDocument.build(screen.doc, name, it.artifacts) } }
    val documents = remember(change?.artifacts) { change?.artifacts.orEmpty().mapNotNull { DocKind.of(it.path) }.distinct().sorted() }
    val viewing = LocalViewing.current
    LaunchedEffect(doc?.path) { doc?.let { viewing(ChatReference("document", it.path, it.label)) } }

    Box(modifier.fillMaxSize()) {
        Column(Modifier.fillMaxSize()) {
            PaneTopBar(
                doc?.title ?: screen.doc.label,
                listOfNotNull(name, change?.revision?.takeIf { it.isNotBlank() }?.let { "revision ${SpecList.shortRevision(it)}" }).joinToString(" · "),
                onBack = { vm.back() }, backLabel = "Back",
            )
            if (documents.isNotEmpty()) DocSelector(
                current = screen.doc, available = documents,
                onPick = { d -> if (d == null) vm.showOverview(screen.changeId) else vm.switchTo(SpecScreen.Document(screen.changeId, d)) },
                modifier = Modifier.padding(bottom = 4.dp),
            )
            when {
                doc != null -> Reader(doc, screen.anchor)
                loaded?.data != null -> MessageState(Icons.Rounded.WarningAmber, "Change unavailable", loaded.data.reason.ifBlank { "The computer couldn't read this change's record." })
                loaded?.error != null -> ErrorState(loaded.error, onRetry = { summary?.let { vm.load(it, force = true) } })
                else -> LoadingState("Loading the document…")
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun Reader(doc: ReaderDocument, anchor: String) {
    if (doc.isEmpty) {
        MessageState(Icons.AutoMirrored.Rounded.FormatListBulleted, "No ${doc.title.lowercase()} yet", "This change doesn't have this document.")
        return
    }
    val list = rememberLazyListState()
    val scope = rememberCoroutineScope()
    val ask = LocalAskAbout.current
    var finding by rememberSaveable(doc.path) { mutableStateOf(false) }
    var query by rememberSaveable(doc.path) { mutableStateOf("") }
    var current by rememberSaveable(doc.path) { mutableIntStateOf(0) }
    var outline by remember { mutableStateOf(false) }
    // The selection: which block and which range of its text.
    var selBlock by remember(doc.path) { mutableStateOf<Int?>(null) }
    var selRange by remember(doc.path) { mutableStateOf(TextRange.Zero) }
    val matches = remember(doc, query) { doc.find(query) }
    val top by remember { derivedStateOf { list.firstVisibleItemIndex } }
    val hasSelection = selBlock != null && !selRange.collapsed

    LaunchedEffect(doc.path, anchor) { doc.indexOf(anchor)?.let { list.scrollToItem(it) } }
    LaunchedEffect(matches, current) { matches.getOrNull(current)?.let { list.animateScrollToItem(it.block) } }

    val selectedText = { selBlock?.let { b -> doc.blocks.getOrNull(b)?.content?.text?.let { t -> t.substring(selRange.min.coerceIn(0, t.length), selRange.max.coerceIn(0, t.length)) } }.orEmpty() }
    val askNow = {
        val ref = doc.reference(if (hasSelection) selBlock else top, if (hasSelection) selectedText() else "")
        selBlock = null; selRange = TextRange.Zero
        ask(ref)
    }
    val askLabel = when {
        hasSelection -> "Ask about the selection"
        doc.blocks.getOrNull(top)?.requirement?.isNotBlank() == true -> "Ask about this requirement"
        else -> "Ask about this document"
    }
    val spec = doc.kind == DocKind.SPECS
    val previous = doc.previous(top)
    val next = doc.next(top)

    Box(Modifier.fillMaxSize()) {
        Column(Modifier.fillMaxSize()) {
            if (finding) PaneSearchField(
                query, { query = it; current = 0 }, "Find in document",
                onClose = { finding = false; query = "" },
                modifier = Modifier.padding(horizontal = 12.dp, vertical = 4.dp),
                trailing = {
                    Text(
                        if (query.isBlank()) "" else if (matches.isEmpty()) "No matches" else "${current + 1} of ${matches.size}",
                        style = MaterialTheme.typography.labelMedium, modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                    )
                    IconButton(onClick = { if (matches.isNotEmpty()) current = (current - 1 + matches.size) % matches.size }, enabled = matches.isNotEmpty(), modifier = Modifier.size(48.dp)) {
                        Icon(Icons.Rounded.KeyboardArrowUp, "Previous match")
                    }
                    IconButton(onClick = { if (matches.isNotEmpty()) current = (current + 1) % matches.size }, enabled = matches.isNotEmpty(), modifier = Modifier.size(48.dp)) {
                        Icon(Icons.Rounded.KeyboardArrowDown, "Next match")
                    }
                },
            )
            LazyColumn(state = list, modifier = Modifier.fillMaxSize(), contentPadding = PaddingValues(start = 24.dp, end = 24.dp, top = 8.dp, bottom = 128.dp)) {
                itemsIndexed(doc.blocks, key = { i, _ -> i }) { i, block ->
                    val here = matches.filter { it.block == i }
                    BlockView(
                        block, isLastStep = block.type == BlockType.STEP && doc.blocks.getOrNull(i + 1)?.type != BlockType.STEP,
                        matches = here, currentMatch = matches.getOrNull(current)?.takeIf { it.block == i },
                        selection = if (selBlock == i) selRange else null,
                        // A selection in one block replaces any other; a tap elsewhere clears it.
                        onSelection = { r -> if (!r.collapsed || selBlock == i) { selBlock = i; selRange = r } else { selBlock = null; selRange = TextRange.Zero } },
                        onAsk = askNow,
                    )
                }
            }
        }

        HorizontalFloatingToolbar(
            expanded = true,
            colors = FloatingToolbarDefaults.vibrantFloatingToolbarColors(),
            floatingActionButton = {
                FloatingToolbarDefaults.VibrantFloatingActionButton(onClick = askNow) { Icon(Icons.AutoMirrored.Rounded.Chat, askLabel) }
            },
            modifier = Modifier.align(Alignment.BottomCenter).padding(bottom = 16.dp),
        ) {
            IconButton(onClick = { outline = true }, modifier = Modifier.size(48.dp)) { Icon(Icons.AutoMirrored.Rounded.FormatListBulleted, "Outline") }
            IconButton(onClick = { finding = true }, modifier = Modifier.size(48.dp)) { Icon(Icons.Rounded.Search, "Find in document") }
            IconButton(onClick = { previous?.let { scope.launch { list.animateScrollToItem(it) } } }, enabled = previous != null, modifier = Modifier.size(48.dp)) {
                Icon(Icons.Rounded.KeyboardArrowUp, if (spec) "Previous change" else "Previous section")
            }
            IconButton(onClick = { next?.let { scope.launch { list.animateScrollToItem(it) } } }, enabled = next != null, modifier = Modifier.size(48.dp)) {
                Icon(Icons.Rounded.KeyboardArrowDown, if (spec) "Next change" else "Next section")
            }
        }
    }

    if (outline) ModalBottomSheet(onDismissRequest = { outline = false }) {
        OutlineSheet(doc) { block -> outline = false; scope.launch { list.scrollToItem(block) } }
    }
}

@Composable
private fun OutlineSheet(doc: ReaderDocument, onPick: (Int) -> Unit) {
    LazyColumn(Modifier.fillMaxWidth(), contentPadding = PaddingValues(start = 16.dp, end = 16.dp, bottom = 32.dp)) {
        item { Text("Outline", style = MaterialTheme.typography.titleLargeEmphasized, modifier = Modifier.padding(8.dp).semantics { heading() }) }
        itemsIndexed(doc.outline, key = { _, e -> e.block }) { _, e ->
            Surface(onClick = { onPick(e.block) }, shape = RoundedCornerShape(16.dp), color = Color.Transparent, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
                Row(Modifier.padding(start = 8.dp + (e.depth * 16).dp, end = 8.dp, top = 10.dp, bottom = 10.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(e.label, style = if (e.depth == 0) MaterialTheme.typography.titleSmall else MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
                    e.delta?.let { DeltaPill(it) }
                }
            }
        }
    }
}

@Composable
private fun BlockView(
    block: ReaderBlock,
    isLastStep: Boolean,
    matches: List<FindMatch>,
    currentMatch: FindMatch?,
    selection: TextRange?,
    onSelection: (TextRange) -> Unit,
    onAsk: () -> Unit,
) {
    val c = MaterialTheme.colorScheme
    val t = MaterialTheme.typography
    val text = styledText(block.content, matches, currentMatch, strike = block.type == BlockType.REQUIREMENT && block.delta == DeltaKind.REMOVED)
    val selectable: @Composable (TextStyle, Modifier, Boolean) -> Unit = { style, m, heading ->
        SelectableText(text, style.copy(color = c.onSurface), selection, onSelection, onAsk, heading, m)
    }
    when (block.type) {
        BlockType.TITLE -> selectable(t.headlineSmallEmphasized, Modifier.padding(top = 8.dp, bottom = 12.dp), true)
        BlockType.HEADING -> selectable(
            when (block.level) { 2 -> t.titleLargeEmphasized; 3 -> t.titleMedium; else -> t.titleSmall },
            Modifier.padding(top = 18.dp, bottom = 6.dp), true,
        )
        BlockType.PARAGRAPH -> selectable(t.bodyLarge, Modifier.padding(bottom = 12.dp), false)
        BlockType.ITEM -> Row(Modifier.padding(start = (block.level * 20).dp, bottom = 6.dp)) {
            Text(block.marker, style = t.bodyLarge, color = c.onSurfaceVariant, modifier = Modifier.width(28.dp).semantics {
                contentDescription = when (block.marker) { "☑" -> "Done"; "☐" -> "Not done"; else -> "" }
            })
            selectable(t.bodyLarge, Modifier.weight(1f), false)
        }
        BlockType.CODE -> Surface(color = c.surfaceContainer, shape = RoundedCornerShape(12.dp), modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp)) {
            Box(Modifier.horizontalScroll(rememberScrollState()).padding(12.dp)) {
                selectable(t.bodyMedium.copy(fontFamily = FontFamily.Monospace), Modifier, false)
            }
        }
        BlockType.QUOTE -> Row(Modifier.padding(bottom = 12.dp)) {
            Box(Modifier.width(4.dp).heightIn(min = 24.dp).background(c.outlineVariant, RoundedCornerShape(2.dp)))
            selectable(t.bodyLarge.copy(fontStyle = FontStyle.Italic), Modifier.padding(start = 12.dp).weight(1f), false)
        }
        BlockType.RULE -> HorizontalDivider(Modifier.padding(vertical = 12.dp))
        BlockType.CAPABILITY -> selectable(t.titleSmall.copy(color = c.onSurfaceVariant), Modifier.padding(top = 20.dp, bottom = 4.dp), true)
        BlockType.REQUIREMENT -> Column(Modifier.padding(top = 16.dp, bottom = 8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            block.delta?.let { DeltaPill(it) }
            selectable(t.titleLargeEmphasized, Modifier, true)
            if (block.from.isNotBlank()) Text("Renamed from \"${block.from}\"", style = t.bodyMedium, color = c.onSurfaceVariant)
        }
        BlockType.SCENARIO -> Surface(color = c.surfaceContainerLow, shape = RoundedCornerShape(20.dp, 20.dp, 4.dp, 4.dp), modifier = Modifier.fillMaxWidth().padding(top = 4.dp)) {
            selectable(t.labelLarge, Modifier.padding(start = 16.dp, end = 16.dp, top = 14.dp, bottom = 4.dp), false)
        }
        BlockType.STEP -> Surface(
            color = c.surfaceContainerLow, shape = if (isLastStep) RoundedCornerShape(4.dp, 4.dp, 20.dp, 20.dp) else RoundedCornerShape(0.dp),
            modifier = Modifier.fillMaxWidth().padding(bottom = if (isLastStep) 12.dp else 0.dp),
        ) {
            selectable(t.bodyMedium, Modifier.padding(start = 16.dp, end = 16.dp, top = 2.dp, bottom = if (isLastStep) 14.dp else 2.dp), false)
        }
    }
}

/** Inline styles, find highlights (the current match stronger), and a strike for a removed requirement. */
@Composable
private fun styledText(content: StyledText, matches: List<FindMatch>, current: FindMatch?, strike: Boolean): AnnotatedString {
    val c = MaterialTheme.colorScheme
    return buildAnnotatedString {
        append(content.text)
        content.spans.forEach { s ->
            val style = when (s.style) {
                InlineStyle.BOLD -> SpanStyle(fontWeight = FontWeight.SemiBold)
                InlineStyle.ITALIC -> SpanStyle(fontStyle = FontStyle.Italic)
                InlineStyle.CODE -> SpanStyle(fontFamily = FontFamily.Monospace, background = c.surfaceContainerHigh)
                InlineStyle.LINK -> SpanStyle(color = c.primary, textDecoration = TextDecoration.Underline)
            }
            addStyle(style, s.start, s.end)
        }
        matches.forEach { m ->
            val on = m == current
            addStyle(SpanStyle(background = if (on) c.tertiary else c.tertiaryContainer, color = if (on) c.onTertiary else c.onTertiaryContainer), m.start, m.end)
        }
        if (strike) addStyle(SpanStyle(textDecoration = TextDecoration.LineThrough), 0, content.text.length)
    }
}

/**
 * Selectable, read-only text whose selection the reader can see, so Ask can attach it. Tab still moves
 * focus on, and the text menu gains "Ask about this".
 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun SelectableText(
    text: AnnotatedString,
    style: TextStyle,
    selection: TextRange?,
    onSelection: (TextRange) -> Unit,
    onAsk: () -> Unit,
    heading: Boolean,
    modifier: Modifier,
) {
    val focus = LocalFocusManager.current
    BasicTextField(
        value = TextFieldValue(text, selection ?: TextRange.Zero),
        onValueChange = { onSelection(it.selection) },
        readOnly = true,
        textStyle = style,
        cursorBrush = SolidColor(Color.Transparent),
        modifier = modifier
            .fillMaxWidth()
            .onPreviewKeyEvent { e ->
                if (e.key == Key.Tab && e.type == KeyEventType.KeyDown) { focus.moveFocus(if (e.isShiftPressed) FocusDirection.Previous else FocusDirection.Next); true } else false
            }
            .appendTextContextMenuComponents { item(key = AskKey, label = "Ask about this") { onAsk(); close() } }
            .semantics { if (heading) heading() },
    )
}

private object AskKey
