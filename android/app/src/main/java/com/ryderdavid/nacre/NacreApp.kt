package com.ryderdavid.nacre

import android.app.Application
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner
import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.claude.BoardAssistant
import com.ryderdavid.nacre.data.claude.ClaudeService
import com.ryderdavid.nacre.data.claude.RepoBoardTools
import com.ryderdavid.nacre.data.settings.SettingsStore
import com.ryderdavid.nacre.notify.RunNotifier
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit

/** Manual dependency container; one per process. */
class AppContainer(app: Application) {
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    val settings = SettingsStore(app)
    val http: OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(8, TimeUnit.SECONDS)
        .readTimeout(45, TimeUnit.SECONDS)
        .writeTimeout(30, TimeUnit.SECONDS)
        .build()
    val repository = NacreRepository(settings, http, scope)
    val claude = ClaudeService()
    val notifier = RunNotifier(app, repository, settings, scope)

    /** One assistant conversation per board, kept for the life of the process. */
    private val assistants = mutableMapOf<String, BoardAssistant>()

    fun assistant(repoKey: String, repoName: String): BoardAssistant = synchronized(assistants) {
        assistants.getOrPut(repoKey) {
            BoardAssistant(claude, { settings.current() }, RepoBoardTools(repository, repoKey, repoName), scope)
        }
    }
}

class NacreApp : Application() {
    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(this)
        container.notifier.start()
        // Live events only while something of ours is on screen; the run watcher covers background runs.
        ProcessLifecycleOwner.get().lifecycle.addObserver(object : DefaultLifecycleObserver {
            override fun onStart(owner: LifecycleOwner) {
                container.repository.startEvents()
                container.repository.refreshRuns()
            }

            override fun onStop(owner: LifecycleOwner) {
                if (!container.notifier.onBackground()) container.repository.stopEvents()
            }
        })
    }
}
