package com.ryderdavid.nacre.notify

import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.ProcessLifecycleOwner
import com.ryderdavid.nacre.NacreApp
import com.ryderdavid.nacre.R
import com.ryderdavid.nacre.data.model.Run
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.launch

/**
 * Keeps the bridge event stream open while Claude Code runs are active so that finished
 * runs can be announced even when Nacre is in the background. Stops itself when idle.
 */
class RunWatcherService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private var watch: Job? = null
    private var refresh: Job? = null

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        running = true
        val container = (application as NacreApp).container
        val repo = container.repository
        val chatWatch = container.chats.watch
        ServiceCompat.startForeground(this, NOTIFICATION_ID, ongoing(runsWorthWatching(repo.runs.value), chatWatch.watching.value.size), ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        repo.startEvents()
        watch?.cancel()
        refresh?.cancel()
        // Learn of running and waiting chats now and on every refresh, so a permission request
        // reaches the phone even when no run is active.
        refresh = scope.launch {
            while (true) {
                chatWatch.refresh()
                delay(CHAT_REFRESH_MS)
            }
        }
        watch = scope.launch {
            delay(1500) // the first refresh of the chat list
            // Watch while runs are active, proposals are being worked on the computer, or chats are at work.
            kotlinx.coroutines.flow.combine(repo.runs, repo.workingChanges, chatWatch.watching) { runs, w, c -> Triple(runs, w, c) }.collectLatest { (runs, w, c) ->
                val active = runsWorthWatching(runs)
                if (!com.ryderdavid.nacre.data.chat.watcherShouldRun(active.size, w.size, c.size)) {
                    delay(1500) // let a follow-up or queued run show up before giving up
                    stopSelf()
                } else if (active.isNotEmpty() || c.isNotEmpty()) {
                    try {
                        NotificationManagerCompat.from(this@RunWatcherService).notify(NOTIFICATION_ID, ongoing(active, c.size))
                    } catch (_: SecurityException) {
                        // Notification permission may be revoked while a run is active.
                        // Keep watching; the foreground service owns its initial notification.
                    }
                }
            }
        }
        return START_NOT_STICKY
    }

    private fun ongoing(active: List<Run>, chats: Int = 0) = NotificationCompat.Builder(this, RunNotifier.CH_ACTIVE)
        .setSmallIcon(R.drawable.ic_stat_nacre)
        .setContentTitle(
            when (active.size) {
                0 -> if (chats > 0) "Watching ${if (chats == 1) "a chat" else "$chats chats"} at work" else "Watching Claude Code runs"
                1 -> "${active[0].agentName} is working on ${active[0].issueId}"
                else -> "${active.size} Claude Code runs in progress"
            }
        )
        .setContentText(active.firstOrNull()?.let { it.lastText.ifBlank { it.issueTitle } } ?: "")
        .setOngoing(true)
        .setSilent(true)
        .setProgress(0, 0, true)
        .setContentIntent(RunNotifier.openRun(this, active.singleOrNull()?.key))
        .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
        .build()

    override fun onDestroy() {
        running = false
        (application as NacreApp).container.chats.watch.stopAll()
        scope.cancel()
        val inForeground = ProcessLifecycleOwner.get().lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)
        if (!inForeground) (application as NacreApp).container.repository.stopEvents()
        super.onDestroy()
    }

    companion object {
        private const val NOTIFICATION_ID = 7420
        /** How often the watcher reads the chat lists again for chats at work. */
        private const val CHAT_REFRESH_MS = 60_000L
        @Volatile var running = false
            private set

        fun start(context: Context) {
            ContextCompat.startForegroundService(context, Intent(context, RunWatcherService::class.java))
        }
    }
}
