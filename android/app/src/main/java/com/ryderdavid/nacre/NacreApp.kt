package com.ryderdavid.nacre

import com.ryderdavid.nacre.data.claude.ChatStack
import android.app.Application
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner
import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.RetiredChats
import com.ryderdavid.nacre.data.settings.SettingsStore
import com.ryderdavid.nacre.notify.RunNotifier
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit

/** Manual dependency container; one per process. */
class AppContainer(app: Application) {
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    val settings = SettingsStore(app)
    val http: OkHttpClient = OkHttpClient.Builder()
        // Event and chat streams each hold a request open; OkHttp's default of 5 per host would queue the rest.
        .dispatcher(okhttp3.Dispatcher().apply { maxRequests = 64; maxRequestsPerHost = 16 })
        .connectTimeout(8, TimeUnit.SECONDS)
        .readTimeout(45, TimeUnit.SECONDS)
        .writeTimeout(30, TimeUnit.SECONDS)
        .build()
    val repository = NacreRepository(settings, http, scope)

    /** The Claude API client for the API key in Settings (checked there; Chat never uses it). */
    val claude = com.ryderdavid.nacre.data.claude.ClaudeService()

    /** The one chat in the app: its current chat id is remembered in settings. */
    val chatStack = ChatStack(persist = { key -> scope.launch { settings.setCurrentChat(key) } })

    private val chatNotifier = com.ryderdavid.nacre.notify.ChatNotifier(app)
    private val inBackground = { !ProcessLifecycleOwner.get().lifecycle.currentState.isAtLeast(androidx.lifecycle.Lifecycle.State.STARTED) }

    /** Chat: conversations, the current chat, the screen-context holder and the background watch. */
    val chats = com.ryderdavid.nacre.data.chat.ChatHub(repository, scope, chatNotifier, inBackground = inBackground, stack = chatStack)

    /** One notification per decision that newly waits, from `/v1/events` and the chats' logs alike. */
    private val decisionAlerts = com.ryderdavid.nacre.data.DecisionAlerts(chatNotifier, inBackground)

    init {
        scope.launch { repository.decisionChanges.collect(decisionAlerts::seen) }
    }

    init {
        scope.launch { chatStack.restore(settings.settings.first().currentChat) }
    }
    val notifier = RunNotifier(app, repository, settings, scope, chats)

    /** Each board's unfinished new-bead form, kept in memory when the form is dismissed. */
    val newBeadDrafts = com.ryderdavid.nacre.data.board.NewBeadDrafts()

    init {
        // The retired bead chat's threads and images: deleted once, retried at the next launch if that fails.
        scope.launch { withContext(Dispatchers.IO) { RetiredChats.cleanUp(app.filesDir) } }
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
