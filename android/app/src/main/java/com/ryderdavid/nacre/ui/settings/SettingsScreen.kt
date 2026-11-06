package com.ryderdavid.nacre.ui.settings

import com.ryderdavid.nacre.data.settings.Host
import com.ryderdavid.nacre.data.HostState
import com.ryderdavid.nacre.data.model.DaemonUpdate
import androidx.compose.material.icons.rounded.SystemUpdateAlt
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.material3.IconButton
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.layout.Box
import android.net.Uri
import android.os.Build
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.Edit
import androidx.compose.material.icons.rounded.ErrorOutline
import androidx.compose.material.icons.rounded.Key
import androidx.compose.material.icons.rounded.Lan
import androidx.compose.material.icons.rounded.Notifications
import androidx.compose.material.icons.rounded.Palette
import androidx.compose.material.icons.rounded.QrCodeScanner
import androidx.compose.material.icons.rounded.AccountTree
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.LargeTopAppBar
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.google.mlkit.common.MlKitException
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning
import com.ryderdavid.nacre.AppContainer
import com.ryderdavid.nacre.BuildConfig
import com.ryderdavid.nacre.data.Connection
import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.claude.CLAUDE_MODELS
import com.ryderdavid.nacre.data.claude.ClaudeService
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.settings.AppSettings
import com.ryderdavid.nacre.data.settings.ThemeMode
import com.ryderdavid.nacre.ui.bead.RUN_MODES
import com.ryderdavid.nacre.ui.common.ClaudeSignInDialog
import com.ryderdavid.nacre.ui.common.SectionHeader
import com.ryderdavid.nacre.ui.common.nacreViewModel
import com.ryderdavid.nacre.ui.theme.Nacre
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** A pairing request from a QR code or a nacre://pair link. */
data class PairRequest(val url: String, val token: String, val name: String, val hostId: String = "") {
    companion object {
        fun parse(raw: String): PairRequest? {
            val uri = runCatching { Uri.parse(raw.trim()) }.getOrNull() ?: return null
            if (uri.scheme != "nacre" || uri.host != "pair") return null
            val url = uri.getQueryParameter("url")?.takeIf { it.startsWith("http") } ?: return null
            val token = uri.getQueryParameter("token")?.takeIf { it.isNotBlank() } ?: return null
            return PairRequest(url, token, uri.getQueryParameter("name").orEmpty(), uri.getQueryParameter("host_id").orEmpty())
        }
    }
}

/** Scan failures that mean this phone has no usable code scanner, so manual entry is the way in. */
private val NO_SCANNER = setOf(
    MlKitException.CODE_SCANNER_UNAVAILABLE,
    MlKitException.CODE_SCANNER_GOOGLE_PLAY_SERVICES_VERSION_TOO_OLD,
    MlKitException.UNAVAILABLE,
)

class SettingsViewModel(private val c: AppContainer) : ViewModel() {
    val settings = c.settings.settings
    val hosts = c.repository.hosts
    val daemonUpdates = c.repository.daemonUpdateStates
    fun refreshDaemonUpdates() = c.repository.refreshDaemonUpdates()
    fun updateComputers() = c.repository.updateConnectedComputers()
    val keyStatus = MutableStateFlow<String?>(null)

    fun update(t: (AppSettings) -> AppSettings) = viewModelScope.launch { c.settings.update(t) }

    /**
     * Adds a computer (or updates one already paired) after checking it answers with
     * this token. Returns an error message, or null on success.
     */
    suspend fun pair(url: String, token: String, name: String): String? {
        val clean = url.trim().trimEnd('/').let { if (it.startsWith("http")) it else "http://$it" }
        BridgeClient.ping(c.http, clean) ?: return "No nacred answered at $clean. Is Tailscale up on both devices?"
        val health = c.repository.probe(clean, token).getOrElse { return it.friendly() }
        c.settings.upsertHost(health.hostId, name.ifBlank { health.hostName.ifBlank { health.hostname } }, clean, token)
        return null
    }

    fun rename(id: String, name: String) = viewModelScope.launch { if (name.isNotBlank()) c.settings.renameHost(id, name) }
    fun remove(id: String) = viewModelScope.launch { c.settings.removeHost(id) }

    fun setKey(key: String) = viewModelScope.launch {
        c.settings.setAnthropicKey(key)
        keyStatus.value = null
    }

    /** Checks the key and model with the Models API (no tokens are spent). */
    fun testKey() = viewModelScope.launch {
        keyStatus.value = "Checking…"
        val s = c.settings.current()
        keyStatus.value = try {
            withContext(Dispatchers.IO) {
                ClaudeService.call { c.claude.client(s.anthropicKey).models().retrieve(s.claudeModel) }
            }
            "Key works with ${CLAUDE_MODELS.firstOrNull { it.first == s.claudeModel }?.second ?: s.claudeModel}"
        } catch (e: Exception) {
            e.message ?: "Key check failed"
        }
    }

    fun reconnect() = c.repository.startEvents()
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(pending: PairRequest?, onPendingHandled: () -> Unit, onBack: () -> Unit) {
    val vm = nacreViewModel { SettingsViewModel(it) }
    val s by vm.settings.collectAsState(initial = null)
    val hosts by vm.hosts.collectAsState()
    val daemonUpdates by vm.daemonUpdates.collectAsState()
    LaunchedEffect(hosts.map { it.host.id to it.online }) { vm.refreshDaemonUpdates() }
    val keyStatus by vm.keyStatus.collectAsState()
    var manual by remember { mutableStateOf(false) }
    var editKey by remember { mutableStateOf(false) }
    var renaming by remember { mutableStateOf<Host?>(null) }
    var removing by remember { mutableStateOf<Host?>(null) }
    var confirm by remember { mutableStateOf(pending) }
    val context = LocalContext.current
    val scroll = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()

    if (pending != null && confirm == null) confirm = pending

    /** Scanning runs in Play services; offer manual entry on phones that can't run it. */
    fun scan() {
        val options = GmsBarcodeScannerOptions.Builder().setBarcodeFormats(Barcode.FORMAT_QR_CODE).enableAutoZoom().build()
        val scanner = runCatching { GmsBarcodeScanning.getClient(context, options) }.getOrNull()
        if (scanner == null) {
            manual = true
            return
        }
        scanner.startScan()
            .addOnSuccessListener { code ->
                confirm = code.rawValue?.let(PairRequest::parse)
                if (confirm == null) manual = true
            }
            .addOnFailureListener { e ->
                val code = (e as? MlKitException)?.errorCode
                if (code != null && code in NO_SCANNER) manual = true
            }
    }

    Scaffold(
        modifier = Modifier.nestedScroll(scroll.nestedScrollConnection),
        topBar = { LargeTopAppBar(title = { Text("Settings") }, scrollBehavior = scroll,
            navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Rounded.ArrowBack, "Back") } }) },
    ) { pad ->
        val p = s ?: return@Scaffold
        LazyColumn(contentPadding = PaddingValues(bottom = 32.dp), modifier = Modifier.padding(pad).fillMaxSize()) {
            item { Header("Your computers") }
            if (hosts.isEmpty()) item {
                ListItem(
                    leadingContent = { Icon(Icons.Rounded.ErrorOutline, null, tint = MaterialTheme.colorScheme.error) },
                    headlineContent = { Text("No computers paired") },
                    supportingContent = { Text("Run `nacred pair` on a Mac and scan the code") },
                )
            }
            items(hosts, key = { it.host.id }) { hs -> HostRow(hs, update = daemonUpdates[hs.host.id], onReconnect = vm::reconnect, onRename = { renaming = hs.host }, onRemove = { removing = hs.host }) }
            item {
                Row(Modifier.padding(horizontal = 16.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(onClick = ::scan) { Icon(Icons.Rounded.QrCodeScanner, null, Modifier.size(18.dp)); Spacer(Modifier.width(8.dp)); Text(if (hosts.isEmpty()) "Scan pairing code" else "Add a computer") }
                    OutlinedButton(onClick = { manual = true }) { Text("Enter manually") }
                }
            }

            if (hosts.isNotEmpty()) item {
                Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    FilledTonalButton(onClick = vm::updateComputers, enabled = hosts.any { it.online } && daemonUpdates.values.none { it.active }) {
                        Icon(Icons.Rounded.SystemUpdateAlt, null, Modifier.size(18.dp))
                        Spacer(Modifier.width(8.dp))
                        Text(when {
                            daemonUpdates.values.any { it.active && it.phase != "loading" } -> "Updating computers…"
                            daemonUpdates.values.any { it.active } -> "Checking computers…"
                            else -> "Update connected computers"
                        })
                    }
                    Text("Installs the latest stable Nacre release on each connected computer. Idle daemons restart; busy computers keep their work.", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }

            item { Header("Refine, Draft and the assistant") }
            item {
                Labeled("Run Claude on", runnerHint(p)) {
                    Segmented(
                        listOf(AppSettings.RUNNER_MAC to "My Mac", AppSettings.RUNNER_API to "API key"),
                        p.claudeRunner,
                    ) { v -> vm.update { it.copy(claudeRunner = v) } }
                }
            }
            item {
                ListItem(
                    leadingContent = { Icon(Icons.Rounded.Key, null) },
                    headlineContent = { Text("API key") },
                    supportingContent = {
                        Text(
                            when {
                                p.apiKeyConfigured -> "sk-ant-…" + p.anthropicKey.takeLast(4) + " · stored encrypted on this phone"
                                p.onMac -> "Optional while Claude runs on your Mac; a fallback when it is offline"
                                else -> "Needed for Refine, Draft and the board assistant"
                            }
                        )
                    },
                    trailingContent = { Icon(Icons.Rounded.Edit, "Change key") },
                    modifier = Modifier.clickable { editKey = true },
                )
            }
            if (p.apiKeyConfigured) item {
                Row(Modifier.padding(horizontal = 16.dp), verticalAlignment = Alignment.CenterVertically) {
                    FilledTonalButton(onClick = vm::testKey) { Text("Test key") }
                    keyStatus?.let { Text(it, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(start = 12.dp)) }
                }
            }
            item {
                var open by remember { mutableStateOf(false) }
                ListItem(
                    headlineContent = { Text("Model") },
                    supportingContent = { Text(CLAUDE_MODELS.firstOrNull { it.first == p.claudeModel }?.second ?: p.claudeModel) },
                    modifier = Modifier.clickable { open = true },
                    trailingContent = {
                        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
                            CLAUDE_MODELS.forEach { (id, label) -> DropdownMenuItem(text = { Text(label) }, onClick = { open = false; vm.update { it.copy(claudeModel = id) } }) }
                        }
                    },
                )
            }
            item {
                Labeled("Effort") {
                    Segmented(listOf("low" to "Low", "medium" to "Medium", "high" to "High"), p.claudeEffort) { v -> vm.update { it.copy(claudeEffort = v) } }
                }
            }

            item { SwitchRow(Icons.Rounded.Notifications, "Notify when runs finish", "While Nacre is in the background", p.notifyRuns) { v -> vm.update { it.copy(notifyRuns = v) } } }

            item { Header("Appearance") }
            item {
                Labeled("Color", "Wallpaper follows your phone's colors. The others are fixed palettes, each with a light and a dark version.") {
                    PalettePicker(
                        current = com.ryderdavid.nacre.ui.theme.Palettes.resolve(p.palette, com.ryderdavid.nacre.ui.theme.Palettes.dynamicAvailable),
                        dynamicAvailable = com.ryderdavid.nacre.ui.theme.Palettes.dynamicAvailable,
                        onPick = { pal -> vm.update { it.copy(palette = pal) } },
                    )
                }
            }
            item { SwitchRow(Icons.Rounded.Notifications, "Chat-first layout", "Projects, then Chat · Spec · Beads. Off shows the classic tabs.", p.chatFirstLayout) { v -> vm.update { it.copy(chatFirstLayout = v) } } }
            item {
                Labeled("Theme") {
                    Segmented(ThemeMode.entries.map { it.name to it.name.lowercase().replaceFirstChar { c -> c.uppercase() } }, p.themeMode.name) { v ->
                        vm.update { it.copy(themeMode = ThemeMode.valueOf(v)) }
                    }
                }
            }

        }

        if (manual) ManualPairDialog(onDismiss = { manual = false }) { req -> manual = false; confirm = req }
        renaming?.let { h -> RenameDialog(h, onDismiss = { renaming = null }) { renaming = null; vm.rename(h.id, it) } }
        removing?.let { h ->
            AlertDialog(
                onDismissRequest = { removing = null },
                title = { Text("Remove ${h.name}?") },
                text = { Text("Its boards and runs disappear from Nacre. nacred keeps running on it; pair again any time.") },
                confirmButton = { TextButton(onClick = { removing = null; vm.remove(h.id) }) { Text("Remove") } },
                dismissButton = { TextButton(onClick = { removing = null }) { Text("Cancel") } },
            )
        }
        if (editKey) KeyDialog(onDismiss = { editKey = false }) { editKey = false; vm.setKey(it) }
        confirm?.let { req ->
            ConfirmPairDialog(req, pair = vm::pair, onDone = { confirm = null; onPendingHandled() })
        }
    }
}

/** Why the chosen runner will or won't work right now. */
private fun runnerHint(p: AppSettings): String = when {
    p.claudeRunner == AppSettings.RUNNER_MAC && !p.bridgeConfigured ->
        "No computer paired yet, so these use the API key below."
    p.claudeRunner == AppSettings.RUNNER_MAC ->
        "Claude Code runs them on your Mac, on your Claude subscription. No API key needed."
    p.apiKeyConfigured -> "The Claude API bills your key for each use."
    else -> "The Claude API bills your key for each use — add one below."
}

@Composable
private fun Header(text: String) = SectionHeader(text, Modifier.padding(horizontal = 16.dp, vertical = 4.dp))

@Composable
private fun Labeled(title: String, subtitle: String? = null, content: @Composable () -> Unit) {
    Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)) {
        Text(title, style = MaterialTheme.typography.bodyLarge)
        if (subtitle != null) Text(subtitle, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Spacer(Modifier.size(8.dp))
        content()
    }
}

@Composable
private fun Segmented(options: List<Pair<String, String>>, selected: String, onPick: (String) -> Unit) {
    SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth().widthIn(max = 560.dp)) {
        options.forEachIndexed { i, (k, l) ->
            SegmentedButton(selected = selected == k, onClick = { onPick(k) }, shape = SegmentedButtonDefaults.itemShape(i, options.size)) { Text(l, maxLines = 1) }
        }
    }
}

@Composable
private fun SwitchRow(icon: androidx.compose.ui.graphics.vector.ImageVector, title: String, sub: String, checked: Boolean, onChange: (Boolean) -> Unit) {
    ListItem(
        leadingContent = { Icon(icon, null) },
        headlineContent = { Text(title) },
        supportingContent = { Text(sub) },
        trailingContent = { Switch(checked = checked, onCheckedChange = onChange) },
        modifier = Modifier.clickable { onChange(!checked) },
    )
}

@Composable
private fun ManualPairDialog(onDismiss: () -> Unit, onNext: (PairRequest) -> Unit) {
    var url by remember { mutableStateOf("http://") }
    var token by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(Icons.Rounded.Lan, null) },
        title = { Text("Connect to nacred") },
        text = {
            Column {
                Text("On the Mac, `nacred token` prints the token.", style = MaterialTheme.typography.bodySmall)
                OutlinedTextField(url, { url = it }, label = { Text("Bridge URL") }, singleLine = true, modifier = Modifier.fillMaxWidth().padding(top = 8.dp))
                OutlinedTextField(token, { token = it }, label = { Text("Token") }, singleLine = true, visualTransformation = PasswordVisualTransformation(), modifier = Modifier.fillMaxWidth().padding(top = 8.dp))
            }
        },
        confirmButton = { Button(onClick = { onNext(PairRequest(url, token, "")) }, enabled = url.isNotBlank() && token.isNotBlank()) { Text("Connect") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun ConfirmPairDialog(req: PairRequest, pair: suspend (String, String, String) -> String?, onDone: () -> Unit) {
    var error by remember { mutableStateOf<String?>(null) }
    var working by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    AlertDialog(
        onDismissRequest = onDone,
        icon = { Icon(Icons.Rounded.Lan, null) },
        title = { Text("Pair with ${req.name.ifBlank { "this Mac" }}?") },
        text = {
            Column {
                Text("Nacre will connect to ${req.url} and use it to read and change your beads and to run Claude Code.")
                error?.let { Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(top = 8.dp)) }
            }
        },
        confirmButton = {
            Button(enabled = !working, onClick = {
                working = true
                scope.launch {
                    error = pair(req.url, req.token, req.name)
                    working = false
                    if (error == null) onDone()
                }
            }) { Text(if (working) "Connecting…" else "Pair") }
        },
        dismissButton = { TextButton(onClick = onDone) { Text("Cancel") } },
    )
}

@Composable
private fun KeyDialog(onDismiss: () -> Unit, onSave: (String) -> Unit) {
    var key by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(Icons.Rounded.Key, null) },
        title = { Text("Claude API key") },
        text = {
            Column {
                Text("Create one at console.anthropic.com. It is encrypted with a key held in this phone's Android Keystore and only sent to the Claude API.", style = MaterialTheme.typography.bodySmall)
                OutlinedTextField(key, { key = it }, label = { Text("sk-ant-…") }, singleLine = true, visualTransformation = PasswordVisualTransformation(), modifier = Modifier.fillMaxWidth().padding(top = 8.dp))
            }
        },
        confirmButton = { Button(onClick = { onSave(key) }, enabled = key.startsWith("sk-")) { Text("Save") } },
        dismissButton = {
            Row {
                TextButton(onClick = { onSave("") }) { Text("Remove") }
                TextButton(onClick = onDismiss) { Text("Cancel") }
            }
        },
    )
}

@Composable
private fun HostRow(hs: HostState, update: DaemonUpdate?, onReconnect: () -> Unit, onRename: () -> Unit, onRemove: () -> Unit) {
    var menu by remember { mutableStateOf(false) }
    var signIn by remember { mutableStateOf(false) }
    if (signIn) ClaudeSignInDialog(hs.host.id, onDismiss = { signIn = false })
    val (sub, ok) = when (val c = hs.connection) {
        is Connection.Online -> listOfNotNull(
            hs.host.url,
            c.health.version.takeIf { it.isNotBlank() }?.let { "nacred $it" },
            c.health.bdVersion.takeIf { it.isNotBlank() }?.let { "bd $it" },
            c.health.claudeVersion.takeIf { it.isNotBlank() }?.let { "Claude Code $it" },
            c.health.codexVersion.takeIf { it.isNotBlank() && c.health.runsCodex }?.let { "Codex $it" },
        ).joinToString(" · ") to true
        Connection.Connecting -> "Connecting… · ${hs.host.url}" to true
        is Connection.Offline -> c.reason to false
        Connection.Unconfigured -> hs.host.url to false
    }
    ListItem(
        leadingContent = {
            Icon(
                if (ok) Icons.Rounded.CheckCircle else Icons.Rounded.ErrorOutline, null,
                tint = if (hs.online) Nacre.colors.doing else if (ok) MaterialTheme.colorScheme.outline else MaterialTheme.colorScheme.error,
            )
        },
        headlineContent = { Text(hs.host.name) },
        supportingContent = {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(sub)
                update?.takeIf { it.phase != "idle" }?.let { state ->
                    Text(state.message, color = if (state.problem) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
                    if (state.active) LinearProgressIndicator(Modifier.fillMaxWidth().padding(top = 4.dp))
                }
            }
        },
        trailingContent = {
            Box {
                IconButton(onClick = { menu = true }) { Icon(Icons.Rounded.MoreVert, "Options for ${hs.host.name}") }
                DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                    DropdownMenuItem(text = { Text("Rename") }, onClick = { menu = false; onRename() })
                    DropdownMenuItem(text = { Text("Reconnect") }, onClick = { menu = false; onReconnect() })
                    DropdownMenuItem(text = { Text("Sign in to Claude") }, enabled = hs.online, onClick = { menu = false; signIn = true })
                    DropdownMenuItem(text = { Text("Remove") }, onClick = { menu = false; onRemove() })
                }
            }
        },
    )
}

@Composable
private fun RenameDialog(host: Host, onDismiss: () -> Unit, onSave: (String) -> Unit) {
    var name by remember { mutableStateOf(host.name) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Rename computer") },
        text = { OutlinedTextField(name, { name = it }, singleLine = true, label = { Text("Name shown in Nacre") }, modifier = Modifier.fillMaxWidth()) },
        confirmButton = { TextButton(onClick = { onSave(name) }, enabled = name.isNotBlank()) { Text("Save") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
