package com.ryderdavid.nacre

import com.ryderdavid.nacre.data.claude.ChatStack
import com.ryderdavid.nacre.data.claude.BeadChats
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
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
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
    /** The one chat in the app: its current chat id is remembered in settings. */
    val chatStack = ChatStack(persist = { key -> scope.launch { settings.setCurrentChat(key) } })

    /** Chat: conversations, the current chat, the screen-context holder and the background watch. */
    val chats = com.ryderdavid.nacre.data.chat.ChatHub(
        repository, scope, com.ryderdavid.nacre.notify.ChatNotifier(app),
        inBackground = { !ProcessLifecycleOwner.get().lifecycle.currentState.isAtLeast(androidx.lifecycle.Lifecycle.State.STARTED) },
        stack = chatStack,
    )

    init {
        scope.launch { chatStack.restore(settings.settings.first().currentChat) }
    }
    val notifier = RunNotifier(app, repository, settings, scope, chats)

    /** Whether a chat can take images on the chosen backend and its target computer; kept current. */
    val imageSupport = com.ryderdavid.nacre.data.claude.ChatImageSupport(settings.settings, repository.hosts)

    /** Each bead's chat with Claude, held on the bead's own screen beside its thread. */
    val beadChats = BeadChats(
        java.io.File(app.filesDir, "bead-chats.json"), scope,
        attachments = com.ryderdavid.nacre.data.claude.ChatAttachments(java.io.File(app.filesDir, "chat-attachments")),
    )

    /** Each board's unfinished new-bead form, kept in memory when the form is dismissed. */
    val newBeadDrafts = com.ryderdavid.nacre.data.board.NewBeadDrafts()

    /**
     * One assistant conversation per board plus the cross-board one, kept for the life
     * of the active chat. Repo keys
     * are "<host>~<repo id>", so [ALL_BOARDS] can't collide.
     */
    private val assistants = mutableMapOf<String, BoardAssistant>()

    fun assistant(repoKey: String, repoName: String): BoardAssistant = synchronized(assistants) {
        assistants.getOrPut(repoKey) {
            BoardAssistant(
                backends, { settings.current() }, SingleBoard(RepoBoardTools(repository, repoKey, repoName)), scope,
                attention = { repository.attention.value },
            )
        }
    }

    /** The conversation on the boards screen, which reaches every board at once. */
    fun orchestrator(): BoardAssistant = synchronized(assistants) {
        assistants.getOrPut(ALL_BOARDS) {
            BoardAssistant(
                backends, { settings.current() }, AllBoardsRouter(repository, settings), scope,
                attention = { repository.attention.value },
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
