package com.ryderdavid.nacre.ui.workspace.spec

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Rule
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material.icons.rounded.Update
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.ryderdavid.nacre.ui.common.ErrorState
import com.ryderdavid.nacre.ui.common.LoadingState
import com.ryderdavid.nacre.ui.common.LocalContainer
import com.ryderdavid.nacre.ui.common.MessageState
import com.ryderdavid.nacre.ui.workspace.segmentShape

/** The Spec destination's first level: changes by group, the specs they touch, and search. */
@Composable
internal fun SpecChangeList(vm: SpecPanelViewModel, modifier: Modifier) {
    val repo = LocalContainer.current.repository
    val board by repo.board(vm.repoKey).collectAsState()
    val rounds by repo.rounds.collectAsState()
    val envelopes by vm.envelopes.collectAsState()
    val query by vm.query.collectAsState()
    val searching by vm.searching.collectAsState()
    val changes = board.data?.changes
    LaunchedEffect(changes) { changes?.let { vm.prefetch(it.changes) } }
    val failed = remember(rounds) { failedBeads(rounds, vm.repoKey) }
    val specPaths = remember(envelopes) { envelopes.mapValues { e -> e.value.data?.change?.artifacts?.map { it.path }.orEmpty() } }
    val accepted = remember(envelopes) { envelopes.values.firstNotNullOfOrNull { it.data?.inventory?.acceptedSpecs } }
    val deltas = remember(envelopes) {
        envelopes.mapValues { e -> e.value.data?.change?.artifacts.orEmpty().mapNotNull { a -> SpecList.capabilityOf(a.path)?.let { SpecDeltas.parse(it, a.content) } } }
    }
    val model = remember(changes, specPaths, accepted, failed, query, deltas) { SpecList.build(changes?.changes.orEmpty(), specPaths, accepted, failed, query, deltas) }

    Column(modifier.fillMaxSize()) {
        PaneTopBar("Spec", if (changes == null) "" else model.countsText, onBack = null) {
            if (!searching) IconButton(onClick = { vm.setSearching(true) }, modifier = Modifier.size(48.dp)) { Icon(Icons.Rounded.Search, "Search specs") }
        }
        if (searching) PaneSearchField(query, vm::setQuery, "Search changes and specs", onClose = { vm.setSearching(false) }, modifier = Modifier.padding(horizontal = 12.dp, vertical = 4.dp))
        when {
            board.data == null && board.error != null -> ErrorState(board.error.orEmpty(), onRetry = { repo.refreshBoard(vm.repoKey) })
            board.data == null -> LoadingState("Loading changes…")
            changes == null -> MessageState(
                Icons.Rounded.Update, "This computer doesn't report changes",
                "Update nacred on this computer to see its OpenSpec changes here.",
            )
            model.changeCount == 0 && model.specs.isEmpty() -> MessageState(
                Icons.AutoMirrored.Rounded.Rule, "No changes yet",
                "Ask in chat to propose a change. It shows here while it's drafted, reviewed and built.",
            )
            else -> SpecListContent(model, onOpen = { vm.open(it) })
        }
    }
}

@Composable
private fun SpecListContent(model: SpecListModel, onOpen: (SpecScreen) -> Unit) {
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(start = 12.dp, end = 12.dp, bottom = 24.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
        if (model.isEmpty) item {
            Text("Nothing matches \"${model.query.trim()}\".", style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(16.dp))
        }
        model.groups.forEach { (group, rows) ->
            item(key = "group:${group.name}") { PaneHeading(group.label, trailing = { Text("${rows.size}", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant) }) }
            itemsIndexed(rows, key = { _, r -> "change:${r.id}" }) { i, row ->
                ChangeRowView(row, segmentShape(i, rows.size)) { onOpen(SpecScreen.Change(row.id)) }
            }
        }
        if (model.requirements.isNotEmpty()) {
            item(key = "requirements") { PaneHeading("Requirements") }
            itemsIndexed(model.requirements, key = { _, r -> "req:${r.changeId}:${r.capability}:${r.requirement}" }) { i, hit ->
                RequirementRowView(hit, segmentShape(i, model.requirements.size)) { onOpen(SpecScreen.Document(hit.changeId, DocKind.SPECS, anchor = hit.requirement)) }
            }
        }
        if (model.specs.isNotEmpty() || model.acceptedSpecs != null) {
            item(key = "specs") {
                PaneHeading("Specs", trailing = { model.acceptedSpecs?.let { Text("$it in the project", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant) } })
                Text(
                    if (model.specs.isEmpty()) "No open change touches a spec right now." else "The specs open changes touch. Each opens at its requirement changes.",
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(start = 8.dp, end = 8.dp, bottom = 6.dp),
                )
            }
            itemsIndexed(model.specs, key = { _, s -> "spec:${s.capability}" }) { i, spec ->
                SpecRowView(spec, segmentShape(i, model.specs.size)) { onOpen(SpecScreen.Document(spec.changeId, DocKind.SPECS, anchor = spec.capability)) }
            }
        }
    }
}

@Composable
private fun ChangeRowView(row: ChangeRow, shape: Shape, onOpen: () -> Unit) {
    val c = MaterialTheme.colorScheme
    val waiting = row.group == SpecGroup.WAITING
    val described = listOfNotNull(
        row.name, row.group.label, row.revision.takeIf { it.isNotBlank() }?.let { "revision $it" }, row.status,
        if (waiting) "Open the queue to answer it" else null,
    ).joinToString(". ")
    Surface(
        onClick = onOpen, shape = shape,
        color = if (waiting) c.tertiaryContainer else c.surfaceContainerLowest,
        contentColor = if (waiting) c.onTertiaryContainer else c.onSurface,
        modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).semantics(mergeDescendants = true) { contentDescription = described },
    ) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 14.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(row.name, style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
                if (row.group == SpecGroup.READY_TO_ARCHIVE) Icon(Icons.Rounded.CheckCircle, null, tint = c.primary, modifier = Modifier.size(20.dp))
                if (row.revision.isNotBlank()) Text("rev ${row.revision}", style = MaterialTheme.typography.labelSmall)
            }
            row.progress?.let { ProgressTrack(it) }
            Text(row.status, style = MaterialTheme.typography.bodySmall, color = if (waiting) c.onTertiaryContainer else c.onSurfaceVariant)
            if (waiting) Text("Waiting for you · open the queue", style = MaterialTheme.typography.labelMedium)
        }
    }
}

@Composable
private fun RequirementRowView(hit: RequirementHit, shape: Shape, onOpen: () -> Unit) {
    Surface(
        onClick = onOpen, shape = shape, color = MaterialTheme.colorScheme.surfaceContainerLowest,
        modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp),
    ) {
        Row(Modifier.padding(horizontal = 16.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(hit.requirement, style = MaterialTheme.typography.titleSmall)
                Text("${hit.capability} · ${hit.changeName}", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            hit.kind?.let { DeltaPill(it) }
        }
    }
}

@Composable
private fun SpecRowView(spec: SpecRow, shape: Shape, onOpen: () -> Unit) {
    Surface(
        onClick = onOpen, shape = shape, color = MaterialTheme.colorScheme.surfaceContainerLowest,
        modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp),
    ) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(spec.capability, style = MaterialTheme.typography.titleSmall)
            Text("Changed by " + spec.changes.joinToString(", "), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}
