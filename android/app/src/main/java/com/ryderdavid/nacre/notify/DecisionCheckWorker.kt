package com.ryderdavid.nacre.notify

import android.content.Context
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.ProcessLifecycleOwner
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.ryderdavid.nacre.NacreApp
import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.settings.Host
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.withTimeoutOrNull
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit

/**
 * Whether the background check should look for decisions now. While the app is on screen or the run
 * watcher holds `/v1/events`, the live stream announces decisions itself.
 */
internal fun decisionCheckDue(notifyRuns: Boolean, computers: Int, streamOpen: Boolean): Boolean =
    notifyRuns && computers > 0 && !streamOpen

/**
 * Every 15 minutes (WorkManager's shortest period, run only with a network), reads the open decisions
 * of each paired computer and announces the pending ones not yet announced, so a Needs you decision
 * raised while nothing keeps the event stream open still reaches the phone. Nothing is read while the
 * stream is open; a computer that doesn't answer is skipped until the next check.
 */
class DecisionCheckWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val container = (applicationContext as? NacreApp)?.container ?: return Result.success()
        val settings = container.settings.current()
        if (!decisionCheckDue(settings.notifyRuns, settings.hosts.size, streamOpen())) return Result.success()
        val open = coroutineScope {
            settings.hosts.map { h -> async { openOn(h, container.http) } }.awaitAll().flatten()
        }
        // The stream may have opened while reading; it announces from here on.
        if (streamOpen()) return Result.success()
        val notices = ChatNotifier(applicationContext)
        container.announcedDecisions.claim(open).forEach(notices::postDecision)
        return Result.success()
    }

    private suspend fun openOn(host: Host, http: OkHttpClient): List<Decision> = withTimeoutOrNull(READ_TIMEOUT_MS) {
        runCatching { BridgeClient(host.url, host.token, http).openDecisions() }
            .getOrElse { e -> if (e is kotlinx.coroutines.CancellationException) throw e; null }
    }.orEmpty().map { it.copy(hostId = host.id) }

    private fun streamOpen(): Boolean =
        RunWatcherService.running || ProcessLifecycleOwner.get().lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)

    companion object {
        private const val WORK_NAME = "decision-check"
        private const val READ_TIMEOUT_MS = 20_000L

        /** Schedules the check while [on] (notifications on and a computer paired), and cancels it otherwise. */
        fun schedule(context: Context, on: Boolean) {
            val work = WorkManager.getInstance(context)
            if (!on) {
                work.cancelUniqueWork(WORK_NAME)
                return
            }
            val request = PeriodicWorkRequestBuilder<DecisionCheckWorker>(15, TimeUnit.MINUTES)
                .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
                .build()
            work.enqueueUniquePeriodicWork(WORK_NAME, ExistingPeriodicWorkPolicy.KEEP, request)
        }
    }
}
