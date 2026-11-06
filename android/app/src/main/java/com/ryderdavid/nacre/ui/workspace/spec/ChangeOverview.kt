package com.ryderdavid.nacre.ui.workspace.spec

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
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
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowForward
import androidx.compose.material.icons.automirrored.rounded.Chat
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.ExpandLess
import androidx.compose.material.icons.rounded.ExpandMore
import androidx.compose.material.icons.rounded.HourglassTop
import androidx.compose.material.icons.rounded.Person
import androidx.compose.material.icons.rounded.PlayArrow
import androidx.compose.material.icons.rounded.Sync
import androidx.compose.material.icons.rounded.Verified
import androidx.compose.material.icons.rounded.WarningAmber
import androidx.compose.material3.ButtonGroupDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.ToggleButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.data.model.DecisionEvidence
import com.ryderdavid.nacre.ui.common.ErrorState
import com.ryderdavid.nacre.ui.common.LoadingState
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.MarkdownText
import com.ryderdavid.nacre.ui.common.MessageState
import com.ryderdavid.nacre.ui.common.relativeTime
import com.ryderdavid.nacre.ui.rounds.usd
import com.ryderdavid.nacre.ui.workspace.ChatReference
import com.ryderdavid.nacre.ui.workspace.LocalAskAbout
import com.ryderdavid.nacre.ui.workspace.LocalViewing
import com.ryderdavid.nacre.ui.workspace.LocalOpenEvidence
import com.ryderdavid.nacre.ui.workspace.LocalOpenQueue
import com.ryderdavid.nacre.ui.workspace.decisions.DecisionModel
import com.ryderdavid.nacre.ui.workspace.segmentShape

/** A change's overview: phase, revision, review, the approved plan beside live progress, and evidence. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun ChangeOverviewPane(vm: SpecPanelViewModel, screen: SpecScreen.Change, modifier: Modifier) {
    val repo = LocalContainer.current.repository
    val board by repo.board(vm.repoKey).collectAsState()
    val rounds by repo.rounds.collectAsState()
    val envelopes by vm.envelopes.collectAsState()
    val summary = board.data?.changes?.changes?.firstOrNull { it.id == screen.changeId }
    val loaded = envelopes[screen.changeId]
    val change = loaded?.data?.change
    LaunchedEffect(summary?.updatedAt) { summary?.let { vm.load(it) } }
    val projectRounds = remember(rounds) { rounds.filter { it.repoKey == vm.repoKey } }
    val overview = remember(change, summary, projectRounds) { change?.let { ChangeOverviewModel.build(it, summary, projectRounds) } }
    val viewing = LocalViewing.current
    val ask = LocalAskAbout.current
    val name = overview?.name ?: summary?.name ?: screen.changeId
    LaunchedEffect(name) { viewing(ChatReference("change", name, name)) }
    // Beads open read-only here; rounds and runs open in the workspace's work inspector.
    var bead by remember { mutableStateOf<String?>(null) }
    val openEvidence = LocalOpenEvidence.current
    // Waiting for you links to the change's decision card: the queue opens at it.
    val decisions by repo.decisions.collectAsState()
    val openQueue = LocalOpenQueue.current
    val seeds = listOfNotNull(change?.seedId).plus(summary?.seedIds.orEmpty()).filter { it.isNotBlank() }.toSet()
    val decision = DecisionModel.forChange(decisions[vm.repoKey].orEmpty(), screen.changeId, change?.changeName ?: summary?.changeName.orEmpty(), seeds)
    val onWaiting: () -> Unit = { repo.focusQueue(vm.repoKey, decision?.id); openQueue() }
    val onEvidence: (EvidenceLink) -> Unit = { link ->
        when (link.kind) {
            EvidenceKind.BEAD -> bead = link.id
            EvidenceKind.ROUND -> openEvidence(DecisionEvidence(link.label, "round", link.id))
            EvidenceKind.RUN -> openEvidence(DecisionEvidence(link.label, "run", link.id))
        }
    }

    Column(modifier.fillMaxSize()) {
        PaneTopBar(name, overview?.revisionText.orEmpty(), onBack = { vm.back() }, backLabel = "Back to changes") {
            IconButton(onClick = { ask(ChatReference("change", name, name)) }, modifier = Modifier.size(48.dp)) {
                Icon(Icons.AutoMirrored.Rounded.Chat, "Ask about this change")
            }
        }
        when {
            overview != null && change != null -> OverviewContent(
                overview, costUsd = change.costUsd, updatedAt = change.updatedAt,
                onDoc = { vm.open(SpecScreen.Document(screen.changeId, it)) },
                onEvidence = onEvidence,
                waitingOn = decision?.title, onWaiting = onWaiting,
                onAskSection = { row -> ask(ChatReference("bead", row.beadId, "${row.beadId} · ${row.title}")) },
            )
            loaded?.data != null && change == null -> MessageState(
                Icons.Rounded.WarningAmber, "Change unavailable", loaded.data.reason.ifBlank { "The computer couldn't read this change's record." },
            )
            loaded?.error != null -> ErrorState(loaded.error, onRetry = { summary?.let { vm.load(it, force = true) } })
            summary == null && board.data != null -> MessageState(Icons.Rounded.WarningAmber, "No longer listed", "This change isn't among the project's open changes any more. It may have been archived.")
            else -> LoadingState("Loading the change…")
        }
    }

    bead?.let { id -> ModalBottomSheet(onDismissRequest = { bead = null }) { BeadEvidence(vm.repoKey, id) } }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun OverviewContent(
    o: ChangeOverview,
    costUsd: Double,
    updatedAt: String,
    onDoc: (DocKind) -> Unit,
    onEvidence: (EvidenceLink) -> Unit,
    /** The title of the decision the change waits on, when the queue has it. */
    waitingOn: String?,
    onWaiting: () -> Unit,
    onAskSection: (PlanRow) -> Unit,
) {
    val c = MaterialTheme.colorScheme
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(start = 12.dp, end = 12.dp, bottom = 32.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item(key = "hero") { Hero(o) }
        if (o.waiting) item(key = "waiting") {
            Banner(
                Icons.Rounded.HourglassTop,
                if (waitingOn != null) "Waiting for you · open its decision" else "Waiting for you · open the queue",
                (waitingOn?.let { "$it. " } ?: "") + "Approve, revise or answer it on its decision card. This panel only reads.",
                c.tertiaryContainer, c.onTertiaryContainer,
                onClick = onWaiting,
            )
        }
        o.stale?.let { item(key = "stale") { Banner(Icons.Rounded.WarningAmber, "Stale", it, c.errorContainer, c.onErrorContainer) } }
        o.error?.let { item(key = "error") { Banner(Icons.Rounded.WarningAmber, "Stopped", it, c.errorContainer, c.onErrorContainer) } }
        if (o.documents.isNotEmpty()) item(key = "docs") { DocSelector(current = null, available = o.documents, onPick = { it?.let(onDoc) }) }

        if (o.plan.isNotEmpty()) {
            item(key = "plan-head") {
                PaneHeading(o.planTitle, trailing = { if (o.approved) Text("Progress from beads", style = MaterialTheme.typography.labelMedium, color = c.onSurfaceVariant) })
                if (!o.approved) Text("Progress shows here from the beads once the plan is approved.", style = MaterialTheme.typography.bodySmall, color = c.onSurfaceVariant, modifier = Modifier.padding(horizontal = 8.dp))
            }
            itemsIndexed(o.plan, key = { _, r -> "section:${r.sectionId}" }) { i, row ->
                PlanRowView(row, segmentShape(i, o.plan.size), onOpen = { row.link?.let(onEvidence) }, onAsk = { onAskSection(row) })
            }
        }

        item(key = "review") { ReviewBlock(o.review) }

        if (o.evidence.isNotEmpty()) item(key = "evidence") {
            PaneHeading("Evidence")
            FlowRow(Modifier.padding(horizontal = 4.dp), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                o.evidence.forEach { link -> EvidenceChip(link) { onEvidence(link) } }
            }
        }
        item(key = "footer") {
            Text(
                listOfNotNull("${usd(costUsd)} spent planning", updatedAt.takeIf { it.isNotBlank() }?.let { "updated ${relativeTime(it)}" }).joinToString(" · "),
                style = MaterialTheme.typography.labelSmall, color = c.onSurfaceVariant, modifier = Modifier.padding(horizontal = 8.dp, vertical = 8.dp),
            )
        }
    }
}

@Composable
private fun Hero(o: ChangeOverview) {
    val c = MaterialTheme.colorScheme
    Surface(shape = RoundedCornerShape(32.dp), color = c.primaryContainer, contentColor = c.onPrimaryContainer, modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(horizontal = 20.dp, vertical = 18.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            @OptIn(ExperimentalLayoutApi::class)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalArrangement = Arrangement.spacedBy(6.dp), itemVerticalAlignment = Alignment.CenterVertically) {
                SpecPill(o.group?.label ?: "Archived", c.primary, c.onPrimary)
            }
            // A short headline gets display type; a long first sentence reads as a title.
            Text(o.headline, style = if (o.headline.length <= 60) MaterialTheme.typography.headlineSmallEmphasized else MaterialTheme.typography.titleLargeEmphasized, modifier = Modifier.semantics { heading() })
            Text(o.status, style = MaterialTheme.typography.bodyMedium)
            o.progress?.let { ProgressTrack(it, height = 8) }
            Text(o.summaryLine, style = MaterialTheme.typography.bodyMedium)
            // The full summary is markdown body text, folded so the plan stays near the top.
            if (o.about.isNotBlank() && o.about != o.headline) {
                var more by androidx.compose.runtime.saveable.rememberSaveable(o.id) { androidx.compose.runtime.mutableStateOf(false) }
                if (more) MarkdownText(o.about)
                androidx.compose.material3.TextButton(onClick = { more = !more }, modifier = Modifier.heightIn(min = 48.dp)) {
                    Text(if (more) "Less" else "What this change does", color = c.onPrimaryContainer)
                }
            }
        }
    }
}

@Composable
private fun Banner(icon: ImageVector, title: String, text: String, container: Color, content: Color, onClick: (() -> Unit)? = null) {
    val body: @Composable () -> Unit = {
        Row(Modifier.padding(horizontal = 16.dp, vertical = 12.dp), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Icon(icon, null, Modifier.size(20.dp))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(title, style = MaterialTheme.typography.labelLarge)
                Text(text, style = MaterialTheme.typography.bodyMedium)
            }
            if (onClick != null) Icon(Icons.AutoMirrored.Rounded.ArrowForward, null, Modifier.size(20.dp).align(Alignment.CenterVertically))
        }
    }
    if (onClick == null) {
        Surface(shape = RoundedCornerShape(20.dp), color = container, contentColor = content, modifier = Modifier.fillMaxWidth().semantics(mergeDescendants = true) {}, content = body)
    } else {
        Surface(
            onClick = onClick, shape = RoundedCornerShape(20.dp), color = container, contentColor = content,
            modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp),
            content = body,
        )
    }
}

/**
 * The document selector: a connected button group (Overview, Proposal, Design, Tasks, Specs). It scrolls
 * sideways rather than truncate a label at large font sizes.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
internal fun DocSelector(current: DocKind?, available: List<DocKind>, onPick: (DocKind?) -> Unit, modifier: Modifier = Modifier) {
    val items: List<DocKind?> = listOf<DocKind?>(null) + available
    Row(
        modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 4.dp).semantics { contentDescription = "Documents" },
        horizontalArrangement = Arrangement.spacedBy(ButtonGroupDefaults.ConnectedSpaceBetween),
    ) {
        items.forEachIndexed { i, d ->
            val label = d?.label ?: "Overview"
            ToggleButton(
                checked = d == current, onCheckedChange = { if (d != current) onPick(d) },
                shapes = when (i) {
                    0 -> ButtonGroupDefaults.connectedLeadingButtonShapes()
                    items.lastIndex -> ButtonGroupDefaults.connectedTrailingButtonShapes()
                    else -> ButtonGroupDefaults.connectedMiddleButtonShapes()
                },
                modifier = Modifier.heightIn(min = 48.dp).semantics { stateDescription = if (d == current) "Showing" else "" },
            ) { Text(label) }
        }
    }
}

@Composable
private fun PlanRowView(row: PlanRow, shape: Shape, onOpen: () -> Unit, onAsk: () -> Unit) {
    val c = MaterialTheme.colorScheme
    val describe = buildList {
        add("Section ${row.number}, ${row.title}")
        if (row.subtitle.isNotBlank()) add(row.subtitle)
        row.state?.let { add(it.label) }
        if (row.note.isNotBlank()) add(row.note)
        row.link?.let { add("Opens ${it.label}") }
    }.joinToString(". ")
    val actions = if (row.beadId.isNotBlank()) listOf(CustomAccessibilityAction("Ask about this section") { onAsk(); true }) else emptyList()
    Surface(
        onClick = onOpen, enabled = row.link != null, shape = shape, color = c.surfaceContainerLowest,
        modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).semantics(mergeDescendants = true) { contentDescription = describe; customActions = actions },
    ) {
        Row(Modifier.padding(horizontal = 16.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Text("${row.number}", style = MaterialTheme.typography.titleSmall, color = c.onSurfaceVariant, modifier = Modifier.width(20.dp))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(row.title, style = MaterialTheme.typography.bodyLarge)
                if (row.subtitle.isNotBlank()) Text(row.subtitle, style = MaterialTheme.typography.bodySmall, color = c.onSurfaceVariant)
                if (row.note.isNotBlank()) Text(row.note, style = MaterialTheme.typography.bodySmall, color = if (row.state == SectionState.FAILED) c.error else c.onSurfaceVariant)
            }
            row.state?.let { StatePill(it) }
        }
    }
}

/** The live state of a section: filled and labelled, distinct from the plan's document-like rows. */
@Composable
private fun StatePill(state: SectionState) {
    val c = MaterialTheme.colorScheme
    when (state) {
        SectionState.LANDED -> SpecPill(state.label, c.primaryContainer, c.onPrimaryContainer, Icons.Rounded.Check)
        SectionState.VERIFIED -> SpecPill(state.label, c.secondaryContainer, c.onSecondaryContainer, Icons.Rounded.Verified)
        SectionState.WORKING -> SpecPill(state.label, c.tertiaryContainer, c.onTertiaryContainer, Icons.Rounded.Sync)
        SectionState.FAILED -> SpecPill(state.label, c.errorContainer, c.onErrorContainer, Icons.Rounded.Close)
        SectionState.CHECK -> SpecPill(state.label, c.tertiaryContainer, c.onTertiaryContainer, Icons.Rounded.Person)
        SectionState.WAITING -> SpecPill(state.label, c.surfaceContainerHighest, c.onSurfaceVariant)
    }
}

@Composable
private fun ReviewBlock(review: ReviewSummary) {
    val c = MaterialTheme.colorScheme
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        PaneHeading("Independent review", trailing = {
            when (review.tone) {
                ReviewTone.PASSED -> SpecPill(review.verdict, c.primaryContainer, c.onPrimaryContainer, Icons.Rounded.Check)
                ReviewTone.CHANGES -> SpecPill(review.verdict, c.errorContainer, c.onErrorContainer, Icons.Rounded.WarningAmber)
                ReviewTone.PENDING -> SpecPill(review.verdict, c.tertiaryContainer, c.onTertiaryContainer, Icons.Rounded.Sync)
                ReviewTone.NONE -> SpecPill(review.verdict, c.surfaceContainerHighest, c.onSurfaceVariant)
            }
        })
        Text(
            when {
                review.findings.isEmpty() && review.tone == ReviewTone.PASSED -> "No findings."
                review.findings.isEmpty() -> "No findings yet."
                else -> review.detail.replaceFirstChar { it.uppercase() } + "."
            },
            style = MaterialTheme.typography.bodySmall, color = c.onSurfaceVariant, modifier = Modifier.padding(start = 8.dp, end = 8.dp, bottom = 6.dp),
        )
        review.findings.forEachIndexed { i, f -> FindingView(f, segmentShape(i, review.findings.size)) }
    }
}

@Composable
private fun FindingView(f: FindingRow, shape: Shape) {
    val c = MaterialTheme.colorScheme
    var open by rememberSaveable(f.id) { mutableStateOf(false) }
    Surface(
        onClick = { open = !open }, shape = shape, color = c.surfaceContainerLowest,
        modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).semantics { stateDescription = if (open) "Expanded" else "Collapsed" },
    ) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                @OptIn(ExperimentalLayoutApi::class)
                FlowRow(Modifier.weight(1f), horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    if (f.blocking) SpecPill("${f.severity} · blocking", c.errorContainer, c.onErrorContainer) else SpecPill(f.severity, c.surfaceContainerHighest, c.onSurfaceVariant)
                    SpecPill(f.disposition, c.secondaryContainer, c.onSecondaryContainer)
                    if (f.needsOwner) SpecPill("Yours to settle", c.tertiaryContainer, c.onTertiaryContainer, Icons.Rounded.Person)
                }
                Icon(if (open) Icons.Rounded.ExpandLess else Icons.Rounded.ExpandMore, null)
            }
            MarkdownText(f.summary)
            if (open) {
                MarkdownText(f.detail.ifBlank { "No more detail." })
                if (f.choices.isNotEmpty()) {
                    Text("Ways to settle it", style = MaterialTheme.typography.labelLarge)
                    f.choices.forEach { Text("• $it", style = MaterialTheme.typography.bodyMedium) }
                }
            }
        }
    }
}

@Composable
private fun EvidenceChip(link: EvidenceLink, onClick: () -> Unit) {
    val icon = when (link.kind) {
        EvidenceKind.ROUND -> Icons.Rounded.PlayArrow
        EvidenceKind.RUN -> Icons.Rounded.Sync
        EvidenceKind.BEAD -> Icons.Rounded.Check
    }
    Box(Modifier.heightIn(min = 48.dp), contentAlignment = Alignment.Center) {
        Surface(onClick = onClick, shape = RoundedCornerShape(20.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh, modifier = Modifier.heightIn(min = 40.dp)) {
            Row(Modifier.padding(horizontal = 14.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                Icon(icon, null, Modifier.size(18.dp))
                Text(link.label, style = MaterialTheme.typography.labelLarge)
            }
        }
    }
}

/** A bead, read-only, as evidence for a section, with Ask about this. */
@Composable
private fun BeadEvidence(repoKey: String, beadId: String) {
    val repo = LocalContainer.current.repository
    val board by repo.board(repoKey).collectAsState()
    val issue = board.data?.index?.byId?.get(beadId)
    val ask = LocalAskAbout.current
    Column(Modifier.fillMaxWidth().padding(start = 24.dp, end = 24.dp, bottom = 32.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text(beadId, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text(issue?.title ?: "Not on the board", style = MaterialTheme.typography.titleLargeEmphasized, modifier = Modifier.semantics { heading() })
        if (issue != null) {
            Text("Status: ${issue.status.replace('_', ' ')}", style = MaterialTheme.typography.bodyMedium)
            if (issue.description.isNotBlank()) MarkdownText(issue.description)
            if (issue.acceptance.isNotBlank()) {
                Text("Acceptance", style = MaterialTheme.typography.titleSmall, modifier = Modifier.semantics { heading() })
                MarkdownText(issue.acceptance)
            }
        }
        Surface(
            onClick = { ask(ChatReference("bead", beadId, beadId)) }, shape = RoundedCornerShape(20.dp),
            color = MaterialTheme.colorScheme.secondaryContainer, contentColor = MaterialTheme.colorScheme.onSecondaryContainer,
            modifier = Modifier.heightIn(min = 48.dp),
        ) {
            Row(Modifier.padding(horizontal = 16.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Icon(Icons.AutoMirrored.Rounded.Chat, null, Modifier.size(18.dp))
                Text("Ask about this", style = MaterialTheme.typography.labelLarge)
            }
        }
    }
}
