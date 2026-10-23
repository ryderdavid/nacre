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

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        running = true
        val repo = (application as NacreApp).container.repository
        ServiceCompat.startForeground(this, NOTIFICATION_ID, ongoing(repo.runs.value.filter { it.isActive }), ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        repo.startEvents()
        watch?.cancel()
        watch = scope.launch {
            repo.runs.collectLatest { runs ->
                val active = runs.filter { it.isActive }
                if (active.isEmpty()) {
                    delay(1500) // let a follow-up or queued run show up before giving up
                    stopSelf()
                } else {
                    runCatching { NotificationManagerCompat.from(this@RunWatcherService).notify(NOTIFICATION_ID, ongoing(active)) }
                }
            }
        }
        return START_NOT_STICKY
    }

    private fun ongoing(active: List<Run>) = NotificationCompat.Builder(this, RunNotifier.CH_ACTIVE)
        .setSmallIcon(R.drawable.ic_stat_nacre)
        .setContentTitle(
            when (active.size) {
                0 -> "Watching Claude Code runs"
                1 -> "Claude is working on ${active[0].issueId}"
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
        scope.cancel()
        val inForeground = ProcessLifecycleOwner.get().lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)
        if (!inForeground) (application as NacreApp).container.repository.stopEvents()
        super.onDestroy()
    }

    companion object {
        private const val NOTIFICATION_ID = 7420
        @Volatile var running = false
            private set

        fun start(context: Context) {
            ContextCompat.startForegroundService(context, Intent(context, RunWatcherService::class.java))
        }
    }
}
