package com.ryderdavid.nacre

import com.ryderdavid.nacre.data.claude.ChatContext
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

    val chatStack = ChatStack(onRemoved = ::resetChat)

    /** Each bead's chat with Claude; accepted messages and replies update the shared stack. */
    val beadChats = BeadChats(
        java.io.File(app.filesDir, "bead-chats.json"), scope,
        onStarted = { key ->
            beadContext(key)?.let { chatStack.started(it, chatLabel(it)) }
        },
        onActivity = { key -> beadContext(key)?.let(chatStack::touch) },
    )

    private fun beadContext(key: String): ChatContext.Bead? {
        val split = key.lastIndexOf('/')
        return if (split < 0) null else ChatContext.Bead(key.substring(0, split), key.substring(split + 1))
    }

    fun chatLabel(context: ChatContext): String = when (context) {
        ChatContext.Multi -> "multi"
        is ChatContext.Board -> repository.repo(context.repoKey)?.name ?: context.repoKey
        is ChatContext.Bead -> repository.board(context.repoKey).value.data?.index?.byId?.get(context.beadId)?.title
            ?.ifBlank { null } ?: context.beadId
    }

    /** Dismissal/expiry starts fresh next time and cancels any work belonging to the old chat. */
    fun resetChat(context: ChatContext) {
        when (context) {
            is ChatContext.Bead -> beadChats.clear(BeadChats.key(context.repoKey, context.beadId))
            else -> synchronized(assistants) {
                assistants.remove(if (context is ChatContext.Board) context.repoKey else ALL_BOARDS)?.clear()
            }
        }
    }

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
                onStarted = { chatStack.started(ChatContext.Board(repoKey), chatLabel(ChatContext.Board(repoKey))) },
                onActivity = { chatStack.touch(ChatContext.Board(repoKey)) },
            )
        }
    }

    /** The conversation on the boards screen, which reaches every board at once. */
    fun orchestrator(): BoardAssistant = synchronized(assistants) {
        assistants.getOrPut(ALL_BOARDS) {
            BoardAssistant(
                backends, { settings.current() }, AllBoardsRouter(repository, settings), scope,
                attention = { repository.attention.value },
                onStarted = { chatStack.started(ChatContext.Multi, "multi") },
                onActivity = { chatStack.touch(ChatContext.Multi) },
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
