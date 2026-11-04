package com.ryderdavid.nacre.ui.bead

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.ExpandLess
import androidx.compose.material.icons.rounded.ExpandMore
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material.icons.automirrored.rounded.Rule
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearWavyProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.compose.ui.window.DialogWindowProvider
import androidx.core.view.WindowCompat
import com.ryderdavid.nacre.data.Loadable
import com.ryderdavid.nacre.data.model.AdoptableChange
import com.ryderdavid.nacre.data.model.ChangeEnvelope
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.SpecChange
import com.ryderdavid.nacre.ui.common.BeadId
import com.ryderdavid.nacre.ui.common.MarkdownText
import com.ryderdavid.nacre.ui.common.SectionHeader
import com.ryderdavid.nacre.ui.common.relativeTime
import com.ryderdavid.nacre.ui.rounds.usd
import kotlinx.coroutines.launch

@Composable
fun ChangeCard(state: Loadable<ChangeEnvelope>, linked: Boolean, onOpen: () -> Unit) {
    val envelope = state.data
    val change = envelope?.change
    val written = if (change == null && !linked) envelope?.suggestedAdoption else null
    SectionHeader("Change")
    Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainer), modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Icon(Icons.AutoMirrored.Rounded.Rule, null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(20.dp))
                Text(change?.let(::changeStatusTitle) ?: if (written != null) "A written change to adopt" else "From intent to reviewed work", style = MaterialTheme.typography.titleSmall)
            }
            Text(
                when {
                    change != null && change.phase == "stale" -> staleText(change.staleReason)
                    change != null -> change.summary.ifBlank { "Review the proposal, decisions and linked implementation work." }
                    written != null -> "${changeTitle(written.changeName)} was written outside Nacre and looks like this bead's change. Adopt it as written to track its sections here."
                    linked -> envelope?.reason?.ifBlank { null } ?: "This bead belongs to a specification workflow. Its record must be available before implementation."
                    envelope != null && !envelope.available -> envelope.reason
                    state.error != null -> "The change status couldn't be loaded. Your bead remains available."
                    state.loading -> "Checking OpenSpec on the computer…"
                    else -> "Compare this bead with existing specifications, develop a proposal, and review it before preparing implementation beads."
                },
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 4, overflow = TextOverflow.Ellipsis,
            )
            if (change != null) Text(
                "${change.sections.size} task sections · ${change.findings.size} review findings",
                style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (change != null || envelope?.available == true || state.error != null || linked) FilledTonalButton(onClick = onOpen) {
                Text(if (change != null || linked) "View change" else if (state.error != null) "Check connection" else if (written != null) "Adopt or develop" else "Develop a proposal")
            }
        }
    }
}

/** The existing detail context opens a focused document, with no new top-level destination. */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun ChangeDetail(
    vm: ChangeViewModel,
    linked: Boolean,
    onDismiss: () -> Unit,
    onOpenBead: (String) -> Unit,
    onOpenRun: (String) -> Unit,
    onOpenRound: (String) -> Unit,
    /** The change as the board knows it: section evidence and found work under way. */
    summary: com.ryderdavid.nacre.data.model.ChangeSummary? = null,
    /** Beads added under the epic while the change was worked. */
    foundWork: List<com.ryderdavid.nacre.data.model.Issue> = emptyList(),
) {
    val state by vm.state.collectAsState()
    val sending by vm.sending.collectAsState()
    val feedback by vm.feedback.collectAsState()
    val draftReady by vm.draftReady.collectAsState()
    val envelope = state.data
    val change = envelope?.change
    val action = change?.let { changeNextAction(it, feedback) }
    val working = change?.isWorking == true || sending
    val canAct = !working && envelope?.available == true && state.error == null
    // A change waiting on Ryder leads with a summary and one recommended step; the files fold into Details.
    val guided = envelope?.available == true && change != null && isGuided(change)
    val choices = change?.takeIf { guided }?.let(::guideChoices)
    // The step he picked, by label; none is the recommendation. "" is his own decision from the chat.
    var picked by rememberSaveable(change?.id, change?.guide?.recommended) { mutableStateOf<String?>(null) }
    val own = com.ryderdavid.nacre.data.model.GuideOption("revise", "Your decision", why = "Drafts and reviews the change again with what you wrote.")
    val chosen = choices?.let { (rec, others) -> if (picked == "") own else others.firstOrNull { it.label == picked } ?: rec }
    var showDetails by rememberSaveable(change?.id) { mutableStateOf(false) }
    val prefs by com.ryderdavid.nacre.ui.common.LocalContainer.current.settings.settings.collectAsState(initial = null)
    // Approving runs the epic, which needs the repository's gates: set them up first, then go on.
    val changeRepos = com.ryderdavid.nacre.ui.common.LocalContainer.current.repository
    var gatesThen by remember { mutableStateOf<(() -> Unit)?>(null) }
    val withGates: (String?, () -> Unit) -> Unit = { action, go ->
        if ((action == "approve" || action == "run") && com.ryderdavid.nacre.ui.rounds.needsGateSetup(changeRepos.repo(vm.repoKey))) gatesThen = go else go()
    }
    gatesThen?.let { go ->
        changeRepos.repo(vm.repoKey)?.let { r ->
            com.ryderdavid.nacre.ui.rounds.GatesSetupSheet(r, onDismiss = { gatesThen = null }, onSaved = { gatesThen = null; go() })
        }
    }
    // Poll until the computer has written the guide.
    LaunchedEffect(guided, change?.guide == null) {
        while (guided && change?.guide == null) { kotlinx.coroutines.delay(4000); vm.refresh() }
    }
    val scope = rememberCoroutineScope()

    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false)) {
        val view = LocalView.current
        val lightSurface = MaterialTheme.colorScheme.surface.luminance() > 0.5f
        SideEffect {
            (view.parent as? DialogWindowProvider)?.window?.let { window ->
                WindowCompat.getInsetsController(window, view).apply {
                    isAppearanceLightStatusBars = lightSurface
                    isAppearanceLightNavigationBars = lightSurface
                }
            }
        }
        Scaffold(
            modifier = Modifier.fillMaxSize(),
            topBar = {
                TopAppBar(
                    title = { Text("Change", maxLines = 1) },
                    navigationIcon = { IconButton(onClick = onDismiss) { Icon(Icons.Rounded.Close, "Close change") } },
                    actions = { IconButton(onClick = { scope.launch { vm.refresh(clearActionError = true) } }, enabled = !state.loading && !sending) { Icon(Icons.Rounded.Refresh, "Refresh change") } },
                )
            },
            bottomBar = {
                if (!guided && (action != null || (envelope?.available == true && change == null && !linked))) {
                    Surface(tonalElevation = 2.dp) {
                        Box(Modifier.fillMaxWidth().navigationBarsPadding().imePadding(), contentAlignment = Alignment.Center) {
                            Column(Modifier.widthIn(max = 840.dp).fillMaxWidth().padding(horizontal = 20.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                                state.error?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error) }
                                Text(
                                    action?.detail ?: "Claude drafts the change and an independent reviewer checks it on your computer until it holds up. You review the result before implementation.",
                                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                                Button(
                                    onClick = {
                                        when (action?.action) {
                                            "run" -> withGates("run") { vm.runEpic { onOpenRound(it.key) } }
                                            "epic" -> onOpenBead(change.epicId)
                                            "review" -> {
                                                val round = change.activeRoundId.ifBlank { change.evidence.lastOrNull { it.roundId.isNotBlank() }?.roundId.orEmpty() }
                                                if (round.isNotBlank()) onOpenRound(Keys.of(Keys.host(vm.repoKey), round))
                                                else change.evidence.lastOrNull { it.runId.isNotBlank() }?.let { onOpenRun(Keys.of(Keys.host(vm.repoKey), it.runId)) }
                                            }
                                            "refresh" -> scope.launch { vm.refresh(clearActionError = true) }
                                            else -> withGates(action?.action) { vm.act(action?.action ?: "develop") }
                                        }
                                    },
                                    enabled = if (action?.action in setOf("review", "epic")) !sending else canAct && draftReady,
                                    modifier = Modifier.fillMaxWidth(),
                                ) { Text(if (sending) "Sending…" else action?.label ?: "Develop a proposal") }
                            }
                        }
                    }
                }
            },
        ) { padding ->
            BoxWithConstraints(Modifier.padding(padding).consumeWindowInsets(padding).fillMaxSize().imePadding(), contentAlignment = Alignment.TopCenter) {
                val wide = guided && maxWidth >= 720.dp
                LazyColumn(
                    modifier = Modifier.widthIn(max = if (wide) 1120.dp else 840.dp).fillMaxSize(),
                    contentPadding = PaddingValues(start = 20.dp, end = 20.dp, bottom = 24.dp),
                    verticalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    if (working || (state.loading && change == null)) item { LinearWavyProgressIndicator(Modifier.fillMaxWidth()) }
                    state.error?.let { error -> item { MessageCard("Couldn't continue", "$error\nThe last loaded proposal is still shown. Refresh before continuing.", error = true) } }
                    if (envelope != null && !envelope.available) item { MessageCard("Workflow unavailable", envelope.reason.ifBlank { "The computer cannot develop this change right now." }) }
                    if (change == null) {
                        item {
                            Text(if (linked) "Recover the linked change" else "Develop this intent", style = MaterialTheme.typography.headlineSmall)
                            Spacer(Modifier.height(8.dp))
                            Text(
                                if (linked) "This bead is linked to a specification workflow, but its saved record is unavailable. Restore the record on the computer before starting implementation."
                                else "Nacre checks project goals, accepted specifications and active proposals, then develops and independently reviews the best fit. Decisions are recorded with the work.",
                                style = MaterialTheme.typography.bodyLarge,
                            )
                        }
                        if (!linked && envelope != null && envelope.adoptable.isNotEmpty()) {
                            item {
                                SectionHeader("Or adopt a change already written")
                                Text(
                                    "Written by hand or in a CLI session. Nacre takes the files as they are, adds section markers to tasks.md, links sections to beads already under this one, and keeps finished sections closed.",
                                    style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                            items(envelope.adoptable, key = { "adoptable:${it.changeName}" }) { written ->
                                AdoptableCard(written, enabled = canAct && draftReady) { recorded -> vm.adopt(written.changeName, recorded) }
                            }
                        }
                    } else {
                        if (guided && choices != null && chosen != null) {
                            item { GuideHeader(change, change.changeName.ifBlank { change.seedId }.let(::changeTitle)) }
                            item {
                                AdaptivePair(wide, { GuideSummary(change) }, {
                                    GuideDecision(
                                        chosen, recommended = chosen == choices.first, seedId = change.seedId,
                                        feedback = feedback, onFeedback = vm::setFeedback,
                                        enabled = canAct && draftReady, sending = sending, onGo = { withGates(chosen.action) { vm.perform(chosen) } },
                                    )
                                })
                            }
                            item {
                                AdaptivePair(wide, {
                                    GuideOptions((listOf(choices.first) + choices.second).filter { it != chosen }, choices.first) { o ->
                                        picked = if (o == choices.first) null else o.label
                                    }
                                }, {
                                    ChangeChat(change, vm.repoKey, prefs?.claudeConfigured == true, busy = !canAct, startOpen = wide) { text ->
                                        vm.setFeedback(text); picked = ""
                                    }
                                })
                            }
                            item {
                                TextButton(onClick = { showDetails = !showDetails }) {
                                    Text(if (showDetails) "Hide details" else "Details: the files, ${change.findings.size} review findings and history")
                                }
                            }
                        }
                        if (!guided || showDetails) {
                        item {
                            Text(changeStatusTitle(change), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary)
                            Spacer(Modifier.height(8.dp))
                            Text(change.changeName.ifBlank { "Developing your change" }.let(::changeTitle), style = MaterialTheme.typography.headlineSmall)
                            if (change.summary.isNotBlank()) Text(change.summary, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.padding(top = 8.dp))
                            if (working) Text("Work continues on the computer when you leave this view.", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 8.dp))
                            if (change.epicId.isNotBlank()) TextButton(onClick = { onOpenBead(change.epicId) }) { Text("Open epic ${change.epicId}") }
                        }
                        if (change.phase in setOf("needs_attention", "stale", "interrupted") || change.error.isNotBlank()) item {
                            MessageCard("Needs attention", change.error.ifBlank {
                                if (change.phase == "stale") staleText(change.staleReason)
                                else action?.detail ?: "Review the saved workflow to continue."
                            }, error = true)
                        }
                        item {
                            ExpandableChangeSection(changeRouteLabel(change.route), "Why this approach", key = "route:${change.id}") {
                                MarkdownText(change.rationale.ifBlank { "The recommendation is being developed." })
                                change.references.forEach { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                            }
                        }
                        items(change.artifacts, key = { "artifact:${it.path}" }) { artifact ->
                            val title = when {
                                artifact.path.endsWith("proposal.md") -> "Proposal"
                                artifact.path.endsWith("design.md") -> "Design"
                                artifact.path.endsWith("tasks.md") -> if (change.hasCurrentApproval) "Plan, as approved" else "Plan"
                                else -> "Behavior · ${artifact.path.substringBeforeLast('/').substringAfterLast('/')}"
                            }
                            val subtitle = if (artifact.path.endsWith("tasks.md") && change.hasCurrentApproval) "Its boxes are the plan, not progress; progress comes from the beads below" else artifact.path
                            ExpandableChangeSection(title, subtitle, "artifact:${change.id}:${artifact.path}") { MarkdownText(artifact.content) }
                        }
                        if (change.findings.isNotEmpty()) {
                            item { SectionHeader("Independent review · ${change.findings.size}") }
                            items(change.findings, key = { "finding:${it.id}" }) { finding ->
                                ExpandableChangeSection(finding.summary, listOf(finding.severity, finding.disposition).filter(String::isNotBlank).joinToString(" · "), "finding:${change.id}:${finding.id}") {
                                    MarkdownText(finding.detail.ifBlank { "No additional detail." })
                                }
                            }
                        }
                        if (change.sections.isNotEmpty() && change.epicId.isNotBlank()) item { PlanProgress(change, summary, foundWork, onOpenBead) }
                        if (change.sections.isNotEmpty()) {
                            item { EvidenceSummary(change) }
                            items(change.sections, key = { "section:${it.id}" }) { section ->
                                val observations = change.evidence.filter { it.sectionId == section.id && it.beadId == section.beadId }
                                ExpandableChangeSection(section.title, section.beadId.ifBlank { "Bead prepared after approval" }, "section:${change.id}:${section.id}") {
                                    MarkdownText(section.body)
                                    if (section.dependsOn.isNotEmpty()) Text("Waits on: ${section.dependsOn.map { id -> change.sections.firstOrNull { it.id == id }?.title ?: id }.joinToString(", ")}", style = MaterialTheme.typography.bodySmall)
                                    if (section.beadId.isNotBlank()) TextButton(onClick = { onOpenBead(section.beadId) }) { Text("Open ${section.beadId}") }
                                    if (observations.isEmpty()) Text("No implementation evidence recorded.", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                    observations.forEach { evidence ->
                                        Text(evidence.note.ifBlank { if (change.hasCurrentApproval && evidence.verified) "Verified implementation recorded." else "Verification has not been established." }, style = MaterialTheme.typography.bodyMedium)
                                        if (evidence.humanCheck) Text("Human check outstanding", color = MaterialTheme.colorScheme.tertiary, style = MaterialTheme.typography.labelMedium)
                                        if (evidence.runId.isNotBlank()) TextButton(onClick = { onOpenRun(Keys.of(Keys.host(vm.repoKey), evidence.runId)) }) { Text("Inspect run") }
                                        if (evidence.roundId.isNotBlank()) TextButton(onClick = { onOpenRound(Keys.of(Keys.host(vm.repoKey), evidence.roundId)) }) { Text(if (evidence.landed) "Inspect merged round" else "Inspect round") }
                                    }
                                }
                            }
                        }
                        if (change.retiredSections.isNotEmpty()) item {
                            ExpandableChangeSection("Retired sections", "Preserved in bead history", "retired:${change.id}") {
                                change.retiredSections.forEach {
                                    Text(it.reason, style = MaterialTheme.typography.bodyMedium)
                                    if (it.beadId.isNotBlank()) TextButton(onClick = { onOpenBead(it.beadId) }) { Text("Open ${it.beadId}") }
                                }
                            }
                        }
                        if (change.history.isNotEmpty()) item {
                            ExpandableChangeSection("Decision history", "${change.history.size} recorded steps", "history:${change.id}") {
                                change.history.asReversed().forEach { entry ->
                                    Text("${entry.kind.replace('_', ' ')} · ${relativeTime(entry.at)}", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                    MarkdownText(entry.text)
                                }
                            }
                        }
                        item {
                            Text(
                                listOfNotNull("${usd(change.costUsd)} used", change.updatedAt.takeIf(String::isNotBlank)?.let { "Updated ${relativeTime(it)}" }, change.revision.takeIf(String::isNotBlank)?.let { "Revision ${it.take(12)}" }).joinToString(" · "),
                                style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline,
                            )
                        }
                        }
                    }
                    if (envelope?.available == true && (change != null || !linked) && !guided) item {
                        SectionHeader(if (change == null) "Additional context" else "Your feedback")
                        OutlinedTextField(
                            value = feedback, onValueChange = vm::setFeedback, enabled = draftReady,
                            label = { Text(if (change == null) "Goals or constraints" else "What should change?") },
                            supportingText = { Text(if (change?.activeRoundId?.isNotBlank() == true) "Saved on this device. Finish the active implementation round before revising the proposal." else "Saved on this device when you leave.") },
                            modifier = Modifier.fillMaxWidth(), shape = RoundedCornerShape(16.dp), minLines = 3,
                        )
                    }
                }
            }
        }
    }
}

private fun changeTitle(name: String) = name.replace('-', ' ').replaceFirstChar { it.uppercase() }

/** One change written outside Nacre, with how adopting it will be reviewed. */
@Composable
private fun AdoptableCard(written: AdoptableChange, enabled: Boolean, onAdopt: (recordedReview: Boolean) -> Unit) {
    var recorded by rememberSaveable(written.changeName) { mutableStateOf(false) }
    Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow), modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(changeTitle(written.changeName), style = MaterialTheme.typography.titleSmall)
            if (written.suggested) Text("Looks like this bead's change", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.primary)
            if (written.why.isNotBlank()) Text(written.why, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 4, overflow = TextOverflow.Ellipsis)
            Text("${written.tasksDone} of ${written.tasksTotal} tasks checked", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (written.problem.isNotBlank()) {
                Text("Can't be adopted as it stands: ${written.problem}", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
            } else {
                if (written.hasReview) Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Use its recorded review.md instead of a new review", style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f).padding(end = 12.dp))
                    Switch(checked = recorded, onCheckedChange = { recorded = it }, enabled = enabled)
                }
                Text(
                    if (recorded) "No AI cost. You still approve before anything changes in Beads."
                    else "One independent review of the files as written. You approve before anything changes in Beads.",
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                FilledTonalButton(onClick = { onAdopt(recorded) }, enabled = enabled) { Text("Adopt as written") }
            }
        }
    }
}

@Composable
private fun EvidenceSummary(change: SpecChange) {
    val current = change.hasCurrentApproval
    val relevant = change.evidence.filter { e -> change.sections.any { it.id == e.sectionId && it.beadId == e.beadId } }
    MessageCard(
        "Implementation evidence",
        buildString {
            append("${change.sections.count { it.beadId.isNotBlank() }} of ${change.sections.size} sections mapped · ${change.verifiedSections} verified runs")
            if (!current) append("\nCurrent approval is required before evidence can verify this change.")
            else {
                append("\n${change.buildVerifiedSections} of ${change.sections.size} sections in a verified build")
                append("\n${relevant.filter { it.humanCheck }.map { it.sectionId }.distinct().size} human checks outstanding")
                append(" · ${relevant.filter { it.landed && it.roundId.isNotBlank() }.map { it.roundId }.distinct().size} merged rounds")
            }
            append("\nThe round checks and merges the work, then finishes the epic into main and archives the spec automatically.")
        },
    )
}

@Composable
private fun MessageCard(title: String, text: String, error: Boolean = false) {
    Surface(color = if (error) MaterialTheme.colorScheme.errorContainer else MaterialTheme.colorScheme.surfaceContainer, shape = RoundedCornerShape(16.dp), modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(title, style = MaterialTheme.typography.titleSmall)
            Text(text, style = MaterialTheme.typography.bodyMedium)
        }
    }
}

@Composable
private fun ExpandableChangeSection(title: String, subtitle: String, key: String, content: @Composable () -> Unit) {
    var expanded by rememberSaveable(key) { mutableStateOf(false) }
    Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow), modifier = Modifier.fillMaxWidth()) {
        Surface(onClick = { expanded = !expanded }, color = MaterialTheme.colorScheme.surfaceContainerLow, modifier = Modifier.fillMaxWidth()) {
            Row(Modifier.padding(16.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Column(Modifier.weight(1f)) {
                    Text(title, style = MaterialTheme.typography.titleSmall)
                    Text(subtitle, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Icon(if (expanded) Icons.Rounded.ExpandLess else Icons.Rounded.ExpandMore, if (expanded) "Collapse $title" else "Expand $title")
            }
        }
        if (expanded) Column(Modifier.padding(start = 16.dp, end = 16.dp, bottom = 16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) { content() }
    }
}

/**
 * Work, from beads: the approved sections in plan order with each bead's
 * evidence, then the beads added under the epic since. tasks.md stays the
 * approved plan; nothing here is ticked back into it.
 */
@Composable
private fun PlanProgress(change: SpecChange, summary: com.ryderdavid.nacre.data.model.ChangeSummary?, found: List<com.ryderdavid.nacre.data.model.Issue>, onOpenBead: (String) -> Unit) {
    val evidence = summary?.sections?.associate { it.id to it.evidence }.orEmpty()
    // After a later revision is approved, sections without fresh evidence need running again.
    val revised = change.history.count { it.kind == "promotion" } > 1
    val landed = change.sections.count { evidence[it.id] == "landed" }
    Surface(color = MaterialTheme.colorScheme.surfaceContainer, shape = RoundedCornerShape(16.dp), modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Work · from beads", style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
                Text("$landed of ${change.sections.size} merged", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            change.sections.forEachIndexed { n, s ->
                val e = evidence[s.id] ?: "none"
                val label = if (e == "none" && revised) "needs a new round" else com.ryderdavid.nacre.ui.board.evidenceLabel(e)
                Row(Modifier.fillMaxWidth().clickable(enabled = s.beadId.isNotBlank()) { onOpenBead(s.beadId) }.padding(vertical = 2.dp), verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text("${n + 1}  ${s.title}", style = MaterialTheme.typography.bodyMedium)
                        if (s.beadId.isNotBlank()) Text(s.beadId, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            if (found.isNotEmpty()) {
                Text("Added during work", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 8.dp))
                found.forEach { f ->
                    val state = when {
                        f.isClosed -> "closed"
                        "found-amend" in f.labels && summary?.pendingFound?.contains(f.id) == true -> "in revision"
                        "found-amend" in f.labels -> "changes done · decide"
                        else -> "open · how/order"
                    }
                    Row(Modifier.fillMaxWidth().clickable { onOpenBead(f.id) }.padding(vertical = 2.dp), verticalAlignment = Alignment.CenterVertically) {
                        Column(Modifier.weight(1f)) {
                            Text(f.title, style = MaterialTheme.typography.bodyMedium)
                            Text(f.id, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                        Text(state, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
            }
        }
    }
}
