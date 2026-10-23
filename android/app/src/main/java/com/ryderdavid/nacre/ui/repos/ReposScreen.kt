package com.ryderdavid.nacre.ui.repos

import com.ryderdavid.nacre.ui.common.Dot
import com.ryderdavid.nacre.data.RepoGroup
import com.ryderdavid.nacre.data.HostState
import androidx.compose.material3.FilterChip
import androidx.compose.material.icons.rounded.Computer
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.Lan
import androidx.compose.material.icons.rounded.PushPin
import androidx.compose.material.icons.rounded.SmartToy
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material.icons.rounded.Terminal
import androidx.compose.material.icons.rounded.VisibilityOff
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LargeTopAppBar
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.ryderdavid.nacre.data.Connection
import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.model.Repo
import com.ryderdavid.nacre.data.settings.SettingsStore
import com.ryderdavid.nacre.ui.common.ErrorState
import com.ryderdavid.nacre.ui.common.LoadingState
import com.ryderdavid.nacre.ui.common.MessageState
import com.ryderdavid.nacre.ui.common.nacreViewModel
import com.ryderdavid.nacre.ui.common.relativeTime
import com.ryderdavid.nacre.ui.theme.MonoFamily
import com.ryderdavid.nacre.ui.theme.Nacre
import kotlinx.coroutines.launch

class ReposViewModel(private val repo: NacreRepository, private val settings: SettingsStore) : ViewModel() {
    val repos = repo.repos
    val groups = repo.groups
    val hosts = repo.hosts
    val prefs = settings.settings

    fun refresh() = repo.refreshRepos(rescan = true)

    fun togglePin(id: String) = viewModelScope.launch {
        settings.update { s -> s.copy(pinnedRepos = if (id in s.pinnedRepos) s.pinnedRepos - id else s.pinnedRepos + id) }
    }

    fun hide(id: String) = viewModelScope.launch { settings.update { s -> s.copy(hiddenRepos = s.hiddenRepos + id) } }
    fun unhideAll() = viewModelScope.launch { settings.update { s -> s.copy(hiddenRepos = emptySet()) } }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ReposScreen(onOpen: (Repo) -> Unit, onPair: () -> Unit) {
    val vm = nacreViewModel { ReposViewModel(it.repository, it.settings) }
    val state by vm.repos.collectAsState()
    val groups by vm.groups.collectAsState()
    val hosts by vm.hosts.collectAsState()
    var hostFilter by rememberSaveable { mutableStateOf<String?>(null) }
    val prefs by vm.prefs.collectAsState(initial = null)
    var query by rememberSaveable { mutableStateOf("") }
    var searching by rememberSaveable { mutableStateOf(false) }
    val scroll = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()

    Scaffold(
        modifier = Modifier.nestedScroll(scroll.nestedScrollConnection),
        topBar = {
            LargeTopAppBar(
                title = {
                    Column {
                        Text("Boards")
                        ConnectionLine(hosts)
                    }
                },
                actions = {
                    IconButton(onClick = { searching = !searching; if (!searching) query = "" }) {
                        Icon(if (searching) Icons.Rounded.Close else Icons.Rounded.Search, "Search boards")
                    }
                    IconButton(onClick = vm::refresh) { Icon(Icons.Rounded.Refresh, "Rescan your computers for beads repos") }
                },
                scrollBehavior = scroll,
            )
        },
    ) { pad ->
        val p = prefs ?: return@Scaffold
        when {
            !p.bridgeConfigured -> MessageState(
                Icons.Rounded.Lan, "Connect a computer",
                "Nacre talks to nacred on your Macs over Tailscale. Run `nacred pair` on one and scan the code.",
                "Pair a computer", onPair, Modifier.padding(pad),
            )
            state.data == null && state.error != null -> ErrorState(state.error!!, vm::refresh, Modifier.padding(pad))
            state.data == null -> LoadingState("Finding beads on your computers…", Modifier.padding(pad))
            else -> {
                val shown = groups.filter { it.id !in p.hiddenRepos }
                val visible = shown
                    .filter { g -> hostFilter == null || g.members.any { it.hostId == hostFilter } }
                    .filter { g -> query.isBlank() || g.home.name.contains(query, true) || g.home.path.contains(query, true) || g.home.gitRemote.contains(query, true) }
                    .sortedWith(
                        compareByDescending<RepoGroup> { it.id in p.pinnedRepos }
                            .thenByDescending { g -> g.members.sumOf { it.activeRuns } > 0 }
                            .thenByDescending { it.home.lastUpdated }
                    )
                PullToRefreshBox(isRefreshing = state.loading, onRefresh = vm::refresh, modifier = Modifier.padding(pad).fillMaxSize()) {
                    LazyVerticalGrid(
                        columns = GridCells.Adaptive(minSize = 320.dp),
                        contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 4.dp, bottom = 24.dp),
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                        verticalArrangement = Arrangement.spacedBy(12.dp),
                        modifier = Modifier.fillMaxSize(),
                    ) {
                        if (searching) item(span = { GridItemSpan(maxLineSpan) }) {
                            OutlinedTextField(query, { query = it }, placeholder = { Text("Filter boards") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                        }
                        item(span = { GridItemSpan(maxLineSpan) }) { Summary(shown) }
                        if (hosts.size > 1) item(span = { GridItemSpan(maxLineSpan) }) {
                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                FilterChip(selected = hostFilter == null, onClick = { hostFilter = null }, label = { Text("All computers") })
                                hosts.forEach { h ->
                                    FilterChip(
                                        selected = hostFilter == h.host.id,
                                        onClick = { hostFilter = if (hostFilter == h.host.id) null else h.host.id },
                                        label = { Text(h.host.name) },
                                        leadingIcon = { Dot(if (h.online) Nacre.colors.doing else MaterialTheme.colorScheme.outline) },
                                    )
                                }
                            }
                        }
                        items(visible, key = { it.id }) { g ->
                            RepoCard(
                                g, showComputers = hosts.size > 1, pinned = g.id in p.pinnedRepos,
                                onOpen = { onOpen(g.home) }, onPin = { vm.togglePin(g.id) }, onHide = { vm.hide(g.id) },
                            )
                        }
                        if (p.hiddenRepos.isNotEmpty()) item(span = { GridItemSpan(maxLineSpan) }) {
                            TextButton(onClick = vm::unhideAll) { Text("Show ${p.hiddenRepos.size} hidden board${if (p.hiddenRepos.size == 1) "" else "s"}") }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ConnectionLine(hosts: List<HostState>) {
    val online = hosts.count { it.online }
    val single = hosts.singleOrNull()
    val (text, color) = when {
        hosts.isEmpty() -> "Not paired" to MaterialTheme.colorScheme.onSurfaceVariant
        single != null -> when (val c = single.connection) {
            is Connection.Online -> "Connected to ${single.host.name}" to Nacre.colors.doing
            is Connection.Offline -> "${single.host.name} offline · ${c.reason}" to MaterialTheme.colorScheme.error
            else -> "Connecting to ${single.host.name}…" to MaterialTheme.colorScheme.onSurfaceVariant
        }
        online == hosts.size -> "${hosts.size} computers connected" to Nacre.colors.doing
        online == 0 -> "No computers reachable" to MaterialTheme.colorScheme.error
        else -> "$online of ${hosts.size} computers connected · " + hosts.filterNot { it.online }.joinToString { it.host.name } + " offline" to Nacre.colors.blocked
    }
    Text(text, style = MaterialTheme.typography.labelMedium, color = color, maxLines = 1, overflow = TextOverflow.Ellipsis)
}

@Composable
private fun Summary(groups: List<RepoGroup>) {
    val repos = groups.map { it.home }
    val open = repos.sumOf { it.counts.open }
    val doing = repos.sumOf { it.counts.inProgress }
    val blocked = repos.sumOf { it.counts.blocked }
    val runs = groups.sumOf { g -> g.members.sumOf { it.activeRuns } }
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth().padding(bottom = 4.dp)) {
        SummaryTile("Open", open, Modifier.weight(1f))
        SummaryTile("In progress", doing, Modifier.weight(1f))
        SummaryTile("Blocked", blocked, Modifier.weight(1f))
        SummaryTile("Running", runs, Modifier.weight(1f), highlight = runs > 0)
    }
}

@Composable
private fun SummaryTile(label: String, n: Int, modifier: Modifier, highlight: Boolean = false) {
    Surface(
        color = if (highlight) MaterialTheme.colorScheme.tertiaryContainer else MaterialTheme.colorScheme.surfaceContainer,
        shape = RoundedCornerShape(16.dp),
        modifier = modifier,
    ) {
        Column(Modifier.padding(horizontal = 12.dp, vertical = 10.dp)) {
            Text("$n", style = MaterialTheme.typography.headlineSmall.copy(fontWeight = FontWeight.SemiBold))
            Text(label, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun RepoCard(g: RepoGroup, showComputers: Boolean, pinned: Boolean, onOpen: () -> Unit, onPin: () -> Unit, onHide: () -> Unit) {
    val r = g.home
    val activeRuns = g.members.sumOf { it.activeRuns }
    // Sessions an agent started itself: the Mac watches them, it does not run them.
    val adoptedRuns = g.members.sumOf { it.adoptedRuns }
    var menu by remember { mutableStateOf(false) }
    Card(
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow),
        shape = RoundedCornerShape(20.dp),
        modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(20.dp)).combinedClickable(onClick = onOpen, onLongClick = { menu = true }),
    ) {
        Column(Modifier.padding(16.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(r.name, style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f, fill = false))
                if (pinned) Icon(Icons.Rounded.PushPin, "Pinned", tint = MaterialTheme.colorScheme.primary, modifier = Modifier.padding(start = 6.dp).size(16.dp))
                Spacer(Modifier.weight(1f))
                if (activeRuns > 0 || adoptedRuns > 0) {
                    Surface(color = MaterialTheme.colorScheme.tertiaryContainer, shape = RoundedCornerShape(8.dp)) {
                        Row(Modifier.padding(horizontal = 8.dp, vertical = 3.dp), verticalAlignment = Alignment.CenterVertically) {
                            Icon(if (activeRuns > 0) Icons.Rounded.Terminal else Icons.Rounded.SmartToy, null, Modifier.size(14.dp))
                            Spacer(Modifier.width(4.dp))
                            Text(
                                listOfNotNull(
                                    activeRuns.takeIf { it > 0 }?.let { "$it running" },
                                    adoptedRuns.takeIf { it > 0 }?.let { "$it agent" },
                                ).joinToString(" · "),
                                style = MaterialTheme.typography.labelSmall,
                            )
                        }
                    }
                }
                Box {
                    DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                        DropdownMenuItem(text = { Text(if (pinned) "Unpin" else "Pin to top") }, leadingIcon = { Icon(Icons.Rounded.PushPin, null) }, onClick = { menu = false; onPin() })
                        DropdownMenuItem(text = { Text("Hide board") }, leadingIcon = { Icon(Icons.Rounded.VisibilityOff, null) }, onClick = { menu = false; onHide() })
                    }
                }
            }
            Text(
                r.path.replace(Regex("^/Users/[^/]+"), "~"),
                style = MaterialTheme.typography.bodySmall.copy(fontFamily = MonoFamily),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1, overflow = TextOverflow.MiddleEllipsis,
            )
            if (showComputers || g.isShared) {
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.padding(top = 8.dp)) {
                    g.members.forEach { m -> ComputerTag(m.hostName, home = g.isShared && m.key == r.key) }
                }
            }
            Spacer(Modifier.height(12.dp))
            if (r.error.isNotBlank()) {
                Text(r.error, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error, maxLines = 2)
            } else {
                StatusBar(r.counts)
                Spacer(Modifier.height(8.dp))
                Row(verticalAlignment = Alignment.CenterVertically) {
                    val c = r.counts
                    Text(
                        listOfNotNull(
                            "${c.open} open",
                            c.inProgress.takeIf { it > 0 }?.let { "$it in progress" },
                            c.blocked.takeIf { it > 0 }?.let { "$it blocked" },
                            "${c.closed} done",
                        ).joinToString(" · "),
                        style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f),
                    )
                    Text(relativeTime(r.lastUpdated), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline)
                }
            }
        }
    }
}

/** A segmented bar showing the share of beads in each state. */
@Composable
fun StatusBar(c: Repo.Counts, modifier: Modifier = Modifier) {
    val total = c.total.coerceAtLeast(1)
    val parts = listOf(
        c.inProgress + c.hooked to Nacre.colors.doing,
        c.open to Nacre.colors.ready,
        c.blocked to Nacre.colors.blocked,
        c.deferred + c.pinned to Nacre.colors.later,
        c.closed to MaterialTheme.colorScheme.outlineVariant,
    ).filter { it.first > 0 }
    Row(modifier.fillMaxWidth().height(8.dp).clip(RoundedCornerShape(4.dp)).background(MaterialTheme.colorScheme.surfaceContainerHighest), horizontalArrangement = Arrangement.spacedBy(2.dp)) {
        parts.forEach { (n, color) -> Box(Modifier.weight(n.toFloat() / total).fillMaxHeight().background(color)) }
    }
}

/** Which computer a board is on; for a repo on several computers, the filled one is home. */
@Composable
fun ComputerTag(name: String, home: Boolean, modifier: Modifier = Modifier) {
    Surface(
        color = if (home) MaterialTheme.colorScheme.secondaryContainer else MaterialTheme.colorScheme.surfaceContainerHighest,
        contentColor = if (home) MaterialTheme.colorScheme.onSecondaryContainer else MaterialTheme.colorScheme.onSurfaceVariant,
        shape = RoundedCornerShape(8.dp),
        modifier = modifier,
    ) {
        Row(Modifier.padding(horizontal = 8.dp, vertical = 3.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Rounded.Computer, null, Modifier.size(13.dp))
            Spacer(Modifier.width(4.dp))
            Text(name, style = MaterialTheme.typography.labelSmall, maxLines = 1)
        }
    }
}
