package com.ryderdavid.nacre.data.settings

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.core.stringSetPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.security.KeyStore
import java.util.UUID
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

private val Context.dataStore: DataStore<Preferences> by preferencesDataStore(name = "nacre")

enum class ThemeMode { SYSTEM, LIGHT, DARK }

/**
 * A paired computer running nacred. [id] is our own stable local id (used in repo and
 * run keys); [hostId] is the id the computer reports, used to recognise re-pairing.
 */
data class Host(val id: String, val hostId: String, val name: String, val url: String, val token: String)

data class AppSettings(
    val hosts: List<Host> = emptyList(),
    /** Per repository (git remote): local id of the computer whose beads the board shows. */
    val homeHosts: Map<String, String> = emptyMap(),
    /** Per repository (git remote): computer last chosen to run beads on. */
    val runHosts: Map<String, String> = emptyMap(),
    val anthropicKey: String = "",
    /** Where Refine, Draft and the board assistant run: [RUNNER_MAC] or [RUNNER_API]. */
    val claudeRunner: String = RUNNER_MAC,
    val claudeModel: String = DEFAULT_MODEL,
    val claudeEffort: String = "medium",
    val runMode: String = "auto",
    val runModel: String = "",
    val runWorktree: Boolean = false,
    val dynamicColor: Boolean = true,
    val themeMode: ThemeMode = ThemeMode.SYSTEM,
    val pinnedRepos: Set<String> = emptySet(),
    val hiddenRepos: Set<String> = emptySet(),
    val detailPaneDp: Int = 440,
    val notifyRuns: Boolean = true,
) {
    val bridgeConfigured get() = hosts.isNotEmpty()
    fun host(id: String) = hosts.firstOrNull { it.id == id }

    /**
     * True when Claude runs on a paired computer, spending Ryder's
     * subscription. Choosing the Mac without one paired falls back to the API
     * key, so the choice never leaves the features dead.
     */
    val onMac get() = claudeRunner == RUNNER_MAC && bridgeConfigured
    val apiKeyConfigured get() = anthropicKey.isNotBlank()

    /** True when the Claude features can work at all. */
    val claudeConfigured get() = onMac || apiKeyConfigured

    companion object {
        const val DEFAULT_MODEL = "claude-opus-5-5"

        /** `claude -p` on a paired computer, on Ryder's Claude subscription. */
        const val RUNNER_MAC = "mac"

        /** The Claude API, billed per use against the key on this phone. */
        const val RUNNER_API = "api"
    }
}

/** Preferences in DataStore; secrets are AES-GCM encrypted with a non-exportable Keystore key. */
class SettingsStore(private val context: Context) {
    private object K {
        // single-bridge settings from 0.1.0, migrated into [hosts] on first read
        val url = stringPreferencesKey("bridge_url")
        val token = stringPreferencesKey("bridge_token_enc")
        val name = stringPreferencesKey("bridge_name")
        val hosts = stringPreferencesKey("hosts_json")
        val homeHosts = stringPreferencesKey("home_hosts_json")
        val runHosts = stringPreferencesKey("run_hosts_json")
        val apiKey = stringPreferencesKey("anthropic_key_enc")
        val runner = stringPreferencesKey("claude_runner")
        val model = stringPreferencesKey("claude_model")
        val effort = stringPreferencesKey("claude_effort")
        val runMode = stringPreferencesKey("run_mode")
        val runModel = stringPreferencesKey("run_model")
        val runWorktree = booleanPreferencesKey("run_worktree")
        val dynamic = booleanPreferencesKey("dynamic_color")
        val theme = stringPreferencesKey("theme_mode")
        val pinned = stringSetPreferencesKey("pinned_repos")
        val hidden = stringSetPreferencesKey("hidden_repos")
        val pane = intPreferencesKey("detail_pane_dp")
        val notify = booleanPreferencesKey("notify_runs")
        val changeFeedback = stringPreferencesKey("change_feedback_json")
    }

    @Serializable
    private data class StoredHost(val id: String, val hostId: String = "", val name: String, val url: String, val tokenEnc: String)

    private val json = Json { ignoreUnknownKeys = true }

    private fun storedHosts(p: Preferences): List<StoredHost> {
        p[K.hosts]?.let { raw -> return runCatching { json.decodeFromString<List<StoredHost>>(raw) }.getOrDefault(emptyList()) }
        // 0.1.0 had one bridge; it becomes the first computer until hosts are next written.
        val url = p[K.url] ?: return emptyList()
        val tok = p[K.token] ?: return emptyList()
        return listOf(StoredHost(LEGACY_ID, "", p[K.name].orEmpty().ifBlank { "Mac" }, url, tok))
    }

    private fun stringMap(raw: String?): Map<String, String> =
        raw?.let { runCatching { json.decodeFromString<Map<String, String>>(it) }.getOrNull() }.orEmpty()

    val settings: Flow<AppSettings> = context.dataStore.data.map { p ->
        AppSettings(
            hosts = storedHosts(p).mapNotNull { h -> Secrets.decrypt(h.tokenEnc)?.let { Host(h.id, h.hostId, h.name, h.url, it) } },
            homeHosts = stringMap(p[K.homeHosts]),
            runHosts = stringMap(p[K.runHosts]),
            anthropicKey = p[K.apiKey]?.let(Secrets::decrypt).orEmpty(),
            claudeRunner = p[K.runner] ?: AppSettings.RUNNER_MAC,
            claudeModel = p[K.model] ?: AppSettings.DEFAULT_MODEL,
            claudeEffort = p[K.effort] ?: "medium",
            runMode = p[K.runMode] ?: "auto",
            runModel = p[K.runModel].orEmpty(),
            runWorktree = p[K.runWorktree] ?: false,
            dynamicColor = p[K.dynamic] ?: true,
            themeMode = p[K.theme]?.let { runCatching { ThemeMode.valueOf(it) }.getOrNull() } ?: ThemeMode.SYSTEM,
            pinnedRepos = p[K.pinned].orEmpty(),
            hiddenRepos = p[K.hidden].orEmpty(),
            detailPaneDp = p[K.pane] ?: 440,
            notifyRuns = p[K.notify] ?: true,
        )
    }

    suspend fun current(): AppSettings = settings.first()

    /** Kept outside AppSettings so a draft does not restart connections or change preferences. */
    suspend fun changeFeedback(key: String): String = stringMap(context.dataStore.data.first()[K.changeFeedback])[key].orEmpty()

    suspend fun setChangeFeedback(key: String, text: String) = context.dataStore.edit { prefs ->
        val drafts = stringMap(prefs[K.changeFeedback])
        prefs[K.changeFeedback] = json.encodeToString(if (text.isEmpty()) drafts - key else drafts + (key to text))
    }

    private suspend fun editHosts(change: (List<StoredHost>) -> List<StoredHost>) = context.dataStore.edit { p ->
        p[K.hosts] = json.encodeToString(change(storedHosts(p)))
        p.remove(K.url); p.remove(K.token); p.remove(K.name)
    }

    /**
     * Adds a computer, or updates it when it is already paired (same reported host id,
     * or same URL). Returns its local id.
     */
    suspend fun upsertHost(hostId: String, name: String, url: String, token: String): String {
        var localId = ""
        editHosts { list ->
            val clean = url.trim().trimEnd('/')
            val existing = list.firstOrNull { (hostId.isNotEmpty() && it.hostId == hostId) || it.url == clean }
            localId = existing?.id ?: "c" + UUID.randomUUID().toString().take(8)
            val h = StoredHost(localId, hostId.ifEmpty { existing?.hostId.orEmpty() }, name.ifBlank { existing?.name ?: "Computer" }, clean, Secrets.encrypt(token.trim()))
            if (existing == null) list + h else list.map { if (it.id == localId) h else it }
        }
        return localId
    }

    /** Records the id a computer reports, so re-pairing it later updates instead of duplicating. */
    suspend fun setReportedHostId(localId: String, hostId: String) = editHosts { list ->
        list.map { if (it.id == localId && it.hostId != hostId) it.copy(hostId = hostId) else it }
    }

    suspend fun renameHost(localId: String, name: String) = editHosts { list -> list.map { if (it.id == localId) it.copy(name = name.trim()) else it } }

    suspend fun removeHost(localId: String) = editHosts { list -> list.filterNot { it.id == localId } }

    suspend fun setHomeHost(groupId: String, localId: String) = context.dataStore.edit {
        it[K.homeHosts] = json.encodeToString(stringMap(it[K.homeHosts]) + (groupId to localId))
    }

    suspend fun setRunHost(groupId: String, localId: String) = context.dataStore.edit {
        it[K.runHosts] = json.encodeToString(stringMap(it[K.runHosts]) + (groupId to localId))
    }

    suspend fun setAnthropicKey(key: String) = context.dataStore.edit {
        if (key.isBlank()) it.remove(K.apiKey) else it[K.apiKey] = Secrets.encrypt(key.trim())
    }

    suspend fun update(transform: (AppSettings) -> AppSettings) {
        val next = transform(current())
        context.dataStore.edit {
            it[K.runner] = next.claudeRunner
            it[K.model] = next.claudeModel
            it[K.effort] = next.claudeEffort
            it[K.runMode] = next.runMode
            it[K.runModel] = next.runModel
            it[K.runWorktree] = next.runWorktree
            it[K.dynamic] = next.dynamicColor
            it[K.theme] = next.themeMode.name
            it[K.pinned] = next.pinnedRepos
            it[K.hidden] = next.hiddenRepos
            it[K.pane] = next.detailPaneDp
            it[K.notify] = next.notifyRuns
        }
    }
}

/** Local id given to the single bridge paired by 0.1.0. */
private const val LEGACY_ID = "c0"

private object Secrets {
    private const val ALIAS = "nacre.secrets.v1"
    private const val TAG_BITS = 128

    private fun key(): SecretKey {
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (ks.getKey(ALIAS, null) as? SecretKey)?.let { return it }
        val gen = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        gen.init(
            KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build()
        )
        return gen.generateKey()
    }

    fun encrypt(plain: String): String {
        val c = Cipher.getInstance("AES/GCM/NoPadding").apply { init(Cipher.ENCRYPT_MODE, key()) }
        val out = c.iv + c.doFinal(plain.toByteArray())
        return Base64.encodeToString(out, Base64.NO_WRAP)
    }

    fun decrypt(enc: String): String? = runCatching {
        val raw = Base64.decode(enc, Base64.NO_WRAP)
        val c = Cipher.getInstance("AES/GCM/NoPadding")
        c.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(TAG_BITS, raw, 0, 12))
        String(c.doFinal(raw, 12, raw.size - 12))
    }.getOrNull()
}
