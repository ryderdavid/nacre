package com.ryderdavid.nacre

import android.app.Application
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner
import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.claude.AllBoardsRouter
import com.ryderdavid.nacre.data.claude.ApiBackend
import com.ryderdavid.nacre.data.claude.BoardAssistant
import com.ryderdavid.nacre.data.claude.ClaudeBackends
import com.ryderdavid.nacre.data.claude.ClaudeService
import com.ryderdavid.nacre.data.claude.MacBackend
import com.ryderdavid.nacre.data.claude.RepoBoardTools
import com.ryderdavid.nacre.data.claude.SingleBoard
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

    /** Refine, Draft and the assistant, on the Mac's subscription or this phone's key. */
    val backends = ClaudeBackends(
        api = ApiBackend(claude) { settings.current() },
        mac = MacBackend(repository) { settings.current() },
    )
    val notifier = RunNotifier(app, repository, settings, scope)

    /**
     * One assistant conversation per board plus the cross-board one, kept for the life
     * of the process (the cross-board one starts fresh after a few idle hours). Repo keys
     * are "<host>~<repo id>", so [ALL_BOARDS] can't collide.
     */
    private val assistants = mutableMapOf<String, BoardAssistant>()

    fun assistant(repoKey: String, repoName: String): BoardAssistant = synchronized(assistants) {
        assistants.getOrPut(repoKey) {
            BoardAssistant(backends, { settings.current() }, SingleBoard(RepoBoardTools(repository, repoKey, repoName)), scope)
        }
    }

    /** The conversation on the boards screen, which reaches every board at once. */
    fun orchestrator(): BoardAssistant = synchronized(assistants) {
        assistants.getOrPut(ALL_BOARDS) {
            BoardAssistant(
                backends, { settings.current() }, AllBoardsRouter(repository, settings), scope,
                idleResetMs = BoardAssistant.ORCHESTRATOR_IDLE_RESET_MS,
            )
        }
    }

    private companion object {
        const val ALL_BOARDS = "@all-boards"
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
