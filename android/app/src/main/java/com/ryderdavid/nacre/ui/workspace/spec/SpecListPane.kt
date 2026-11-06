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
import androidx.compose.material.icons.rounded.ExpandLess
import androidx.compose.material.icons.rounded.ExpandMore
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
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
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
    val inventories by repo.currentSpecs.collectAsState()
    // Every current spec, read again whenever the changes do (an archive changes both).
    LaunchedEffect(changes) { changes?.let { vm.prefetch(it.changes); repo.refreshSpecs(vm.repoKey) } }
    val current = inventories[vm.repoKey]
    val failed = remember(rounds) { failedBeads(rounds, vm.repoKey) }
    val specPaths = remember(envelopes) { envelopes.mapValues { e -> e.value.data?.change?.artifacts?.map { it.path }.orEmpty() } }
    val accepted = remember(envelopes) { envelopes.values.firstNotNullOfOrNull { it.data?.inventory?.acceptedSpecs } }
    val deltas = remember(envelopes) {
        envelopes.mapValues { e -> e.value.data?.change?.artifacts.orEmpty().mapNotNull { a -> SpecList.capabilityOf(a.path)?.let { SpecDeltas.parse(it, a.content) } } }
    }
    val model = remember(changes, specPaths, accepted, failed, query, deltas, current) { SpecList.build(changes?.changes.orEmpty(), specPaths, accepted, failed, query, deltas, current) }

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
                    when {
                        model.allSpecs && model.specs.isEmpty() && model.query.isNotBlank() -> "No spec matches \"${model.query.trim()}\"."
                        model.allSpecs && model.specs.isEmpty() -> "This project has no OpenSpec specs yet."
                        model.allSpecs -> "The project's current specs. One an open change touches opens at its requirement changes."
                        model.specs.isEmpty() -> "No open change touches a spec right now."
                        else -> "The specs open changes touch. Each opens at its requirement changes."
                    },
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(start = 8.dp, end = 8.dp, bottom = 6.dp),
                )
            }
            itemsIndexed(model.specs, key = { _, s -> "spec:${s.capability}" }) { i, spec ->
                SpecRowView(spec, segmentShape(i, model.specs.size), onOpen = spec.changeId?.let { id -> { onOpen(SpecScreen.Document(id, DocKind.SPECS, anchor = spec.capability)) } })
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

/**
 * One spec. One an open change touches opens at that change's requirement changes ([onOpen]); one no
 * change touches expands to its requirements, read-only.
 */
@Composable
private fun SpecRowView(spec: SpecRow, shape: Shape, onOpen: (() -> Unit)?) {
    var expanded by rememberSaveable(spec.capability) { mutableStateOf(false) }
    val c = MaterialTheme.colorScheme
    val line = when {
        spec.touched && !spec.current -> "New in " + spec.changes.joinToString(", ")
        spec.touched -> "Changed by " + spec.changes.joinToString(", ")
        spec.requirements.size == 1 -> "1 requirement"
        else -> "${spec.requirements.size} requirements"
    }
    val canExpand = onOpen == null && spec.requirements.isNotEmpty()
    Surface(
        onClick = { if (onOpen != null) onOpen() else if (canExpand) expanded = !expanded }, enabled = onOpen != null || canExpand,
        shape = shape, color = c.surfaceContainerLowest, contentColor = c.onSurface,
        modifier = Modifier.fillMaxWidth().heightIn(min = 56.dp).semantics(mergeDescendants = true) {
            if (canExpand) stateDescription = if (expanded) "Requirements shown" else "Requirements hidden"
        },
    ) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(spec.capability, style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
                if (canExpand) Icon(if (expanded) Icons.Rounded.ExpandLess else Icons.Rounded.ExpandMore, null, Modifier.size(20.dp), tint = c.onSurfaceVariant)
            }
            Text(line, style = MaterialTheme.typography.bodySmall, color = if (spec.touched) c.primary else c.onSurfaceVariant)
            if (expanded) Column(Modifier.padding(top = 6.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                spec.requirements.forEach { Text("· $it", style = MaterialTheme.typography.bodyMedium) }
            }
        }
    }
}
