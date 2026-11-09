package com.ryderdavid.nacre.notify

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import com.ryderdavid.nacre.data.model.AttentionItem
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.ProcessLifecycleOwner
import com.ryderdavid.nacre.MainActivity
import com.ryderdavid.nacre.R
import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.model.Run
import com.ryderdavid.nacre.data.model.BEAD_PARKED
import com.ryderdavid.nacre.data.model.ROUND_LANDED
import com.ryderdavid.nacre.data.model.ROUND_PAUSED
import com.ryderdavid.nacre.data.model.ROUND_READY
import com.ryderdavid.nacre.data.model.ROUND_NEEDS_DECISION
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.settings.SettingsStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.util.Locale

/**
 * Runs that justify keeping [RunWatcherService] alive. Adopted runs are left out on
 * purpose: the Mac only watches an agent's own session, which may be a terminal someone
 * leaves open for hours, so treating one as work in progress would pin a foreground
 * service to an idle window.
 */
internal fun runsWorthWatching(runs: List<Run>): List<Run> = runs.filter { it.isNacreWork }

/**
 * Whether a run that just reached a terminal state should be announced. An adopted run is
 * never announced: nothing finished for us when a person quits their own session, and the
 * notification would claim a result Nacre never asked for.
 */
/**
 * Runs that have just been flagged stalled, given the keys already announced. A run is
 * announced once per stall: it drops out of [announced] when it prints again or ends.
 */
internal fun newlyStalled(runs: List<Run>, announced: Set<String>): List<Run> =
    runs.filter { it.stalled && it.isNacreWork && it.key !in announced }

/** A notification about a round. */
data class RoundNotice(val title: String, val body: String)

/**
 * What, if anything, a round change should announce: a bead parked, a pause, a landing. Each fires once,
 * on the change into it. A question and a round ready to land are decisions, announced under the one
 * "N things need you" summary, so they aren't announced here too.
 */
internal fun roundNotice(before: Round?, after: Round): RoundNotice? {
    val beads = after.beads.joinToString(", ") { it.id }
    if (before?.state != after.state) when (after.state) {
        ROUND_PAUSED -> return RoundNotice("A round paused", after.pauseReason.ifBlank { "Beads $beads" })
        ROUND_LANDED -> return RoundNotice("Round merged", "${after.target} is at ${after.landedCommit.take(7)}${if (after.pushed) " and pushed" else ""}.")
    }
    val parkedBefore = before?.beads.orEmpty().filter { it.state == BEAD_PARKED }.map { it.id }.toSet()
    after.beads.firstOrNull { it.state == BEAD_PARKED && it.id !in parkedBefore }?.let {
        return RoundNotice("${it.id} was parked", it.reason.ifBlank { "The rest of the round carries on without it." })
    }
    return null
}

/**
 * The Needs you items worth a notification of their own: none. A proposal ready for review and a change
 * ready to archive are decisions (approve, finish), announced under the "N things need you" summary.
 */
@Suppress("UNUSED_PARAMETER")
internal fun changeNotices(items: List<AttentionItem>, announced: Set<String>): List<AttentionItem> = emptyList()

internal fun shouldAnnounceFinish(run: Run, notifyRuns: Boolean, appInForeground: Boolean): Boolean =
    notifyRuns && !appInForeground && !run.isAdopted

/** Posts run-finished notifications and keeps [RunWatcherService] alive while runs are active. */
class RunNotifier(
    private val context: Context,
    private val repository: NacreRepository,
    private val settings: SettingsStore,
    private val scope: CoroutineScope,
    private val chats: com.ryderdavid.nacre.data.chat.ChatHub? = null,
) {
    /** Stalled runs already announced, so each stall notifies once. */
    private var stallsAnnounced: Set<String> = emptySet()

    private val foreground get() = ProcessLifecycleOwner.get().lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)

    fun start() {
        createChannels()
        scope.launch {
            repository.finishedRuns.collect { run ->
                if (shouldAnnounceFinish(run, settings.current().notifyRuns, foreground)) postFinished(run)
            }
        }
        scope.launch {
            repository.roundChanges.collect { (before, after) ->
                if (settings.current().notifyRuns) roundNotice(before, after)?.let { postRound(after, it) }
            }
        }
        scope.launch {
            // The first list only records what is already waiting: it is on the
            // Boards screen, so there is nothing new to announce.
            var announced: Set<String>? = null
            repository.attention.collect { items ->
                if (!repository.attentionSupported.value) return@collect
                val seen = announced
                if (seen != null && settings.current().notifyRuns) changeNotices(items, seen).forEach(::postChange)
                announced = seen.orEmpty() + items.map { it.id }
            }
        }
        scope.launch {
            repository.workingChanges.collect { w ->
                if (w.isNotEmpty() && foreground && !RunWatcherService.running) {
                    withContext(Dispatchers.Main) { runCatching { RunWatcherService.start(context) } }
                }
            }
        }
        scope.launch {
            // A chat at work keeps the watcher running, so a permission request still reaches the phone in the background.
            repository.chats.collect { listing ->
                if (listing.chats.any { it.active } && foreground && !RunWatcherService.running) {
                    withContext(Dispatchers.Main) { runCatching { RunWatcherService.start(context) } }
                }
            }
        }
        scope.launch {
            repository.runs.collect { runs ->
                val fresh = newlyStalled(runs, stallsAnnounced)
                stallsAnnounced = runs.filter { it.stalled && it.isNacreWork }.mapTo(mutableSetOf()) { it.key }
                if (settings.current().notifyRuns) fresh.forEach(::postStalled)
                if (runsWorthWatching(runs).isNotEmpty() && foreground && !RunWatcherService.running) {
                    withContext(Dispatchers.Main) { runCatching { RunWatcherService.start(context) } }
                }
            }
        }
    }

    /** Called when the app leaves the foreground. True if the watcher keeps the event stream. */
    fun onBackground(): Boolean {
        // Learn of running and waiting chats from the chat list as the app leaves the foreground.
        chats?.let { hub ->
            scope.launch {
                hub.watch.refresh()
                if (hub.watch.watching.value.isNotEmpty() && !RunWatcherService.running) {
                    withContext(Dispatchers.Main) { runCatching { RunWatcherService.start(context) } }
                }
            }
        }
        return RunWatcherService.running
    }

    private fun createChannels() {
        val nm = context.getSystemService(NotificationManager::class.java)
        nm.createNotificationChannel(NotificationChannel(CH_ACTIVE, "Claude Code runs in progress", NotificationManager.IMPORTANCE_LOW).apply {
            description = "Shown while a bead is running on your Mac"
        })
        nm.createNotificationChannel(NotificationChannel(CH_DONE, "Finished runs", NotificationManager.IMPORTANCE_DEFAULT).apply {
            description = "When a Claude Code run on your Mac finishes"
        })
        nm.createNotificationChannel(NotificationChannel(CH_STUCK, "Runs that may be stuck", NotificationManager.IMPORTANCE_DEFAULT).apply {
            description = "When a Claude Code run has printed nothing for a long time"
        })
        nm.createNotificationChannel(NotificationChannel(CH_ROUNDS, "Rounds that need you", NotificationManager.IMPORTANCE_DEFAULT).apply {
            description = "When a round needs a decision, parks a bead, pauses, or is ready to merge, and when a proposal is ready for review or a change is ready to archive"
        })
    }

    private fun postRound(round: Round, notice: RoundNotice) {
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) return
        val n = NotificationCompat.Builder(context, CH_ROUNDS)
            .setSmallIcon(R.drawable.ic_stat_nacre)
            .setContentTitle(notice.title)
            .setContentText(notice.body)
            .setStyle(NotificationCompat.BigTextStyle().bigText(notice.body))
            .setSubText(listOfNotNull(round.hostName.ifBlank { null }, round.repoName).joinToString(" · "))
            .setContentIntent(openRound(context, round.key))
            .setAutoCancel(true)
            .setCategory(NotificationCompat.CATEGORY_STATUS)
            .build()
        NotificationManagerCompat.from(context).notify((round.key + notice.title).hashCode(), n)
    }

    private fun postChange(item: AttentionItem) {
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) return
        val title = if (item.kind == AttentionItem.PROPOSAL_READY) "Proposal ready for your review" else "Change ready to archive"
        val intent = Intent(context, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
            putExtra(EXTRA_CHANGE_REPO, item.repoKey)
            putExtra(EXTRA_CHANGE_BEAD, item.beadId)
        }
        val n = NotificationCompat.Builder(context, CH_ROUNDS)
            .setSmallIcon(R.drawable.ic_stat_nacre)
            .setContentTitle(title)
            .setContentText(item.title)
            .setStyle(NotificationCompat.BigTextStyle().bigText(item.title + "\n" + item.why))
            .setSubText(listOfNotNull(item.hostName.ifBlank { null }, item.repoName.ifBlank { null }).joinToString(" · "))
            .setContentIntent(PendingIntent.getActivity(context, item.id.hashCode(), intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT))
            .setAutoCancel(true)
            .setCategory(NotificationCompat.CATEGORY_STATUS)
            .build()
        NotificationManagerCompat.from(context).notify(item.id.hashCode(), n)
    }

    private fun postStalled(run: Run) {
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) return
        val n = NotificationCompat.Builder(context, CH_STUCK)
            .setSmallIcon(R.drawable.ic_stat_nacre)
            .setContentTitle("${run.issueId} may be stuck")
            .setContentText("${run.agentName} has printed nothing for a while. Open it to cancel and follow up.")
            .setSubText(listOfNotNull(run.hostName.ifBlank { null }, run.repoName).joinToString(" · "))
            .setContentIntent(openRun(context, run.key))
            .setAutoCancel(true)
            .setCategory(NotificationCompat.CATEGORY_STATUS)
            .build()
        NotificationManagerCompat.from(context).notify(run.key.hashCode(), n)
    }

    private fun postFinished(run: Run) {
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) return
        val title = when (run.status) {
            "succeeded" -> "${run.issueId} finished"
            "cancelled" -> "${run.issueId} run cancelled"
            else -> "${run.issueId} run failed"
        }
        val body = run.resultText.ifBlank { run.error.ifBlank { run.lastText } }.ifBlank { run.issueTitle }
        val n = NotificationCompat.Builder(context, CH_DONE)
            .setSmallIcon(R.drawable.ic_stat_nacre)
            .setContentTitle(title)
            .setContentText(body.take(200))
            .setStyle(NotificationCompat.BigTextStyle().bigText(body.take(1200)))
            .setSubText(listOfNotNull(run.hostName.ifBlank { null }, run.repoName, cost(run)).joinToString(" · "))
            .setContentIntent(openRun(context, run.key))
            .setAutoCancel(true)
            .setCategory(NotificationCompat.CATEGORY_STATUS)
            .build()
        NotificationManagerCompat.from(context).notify(run.key.hashCode(), n)
    }

    companion object {
        const val CH_ACTIVE = "runs_active"
        const val CH_DONE = "runs_done"
        const val CH_STUCK = "runs_stuck"
        const val EXTRA_RUN = "nacre.run_id"
        const val EXTRA_ROUND = "nacre.round_id"
        const val EXTRA_CHANGE_REPO = "nacre.change_repo"
        const val EXTRA_CHANGE_BEAD = "nacre.change_bead"
        const val CH_ROUNDS = "rounds"
        const val EXTRA_CHAT = "nacre.chat_key"
        const val EXTRA_CHAT_REQUEST = "nacre.chat_request"

        fun openRound(context: Context, roundKey: String): PendingIntent {
            val intent = Intent(context, MainActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
                putExtra(EXTRA_ROUND, roundKey)
            }
            return PendingIntent.getActivity(context, roundKey.hashCode(), intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        }

        fun cost(run: Run): String? = if (run.costUsd > 0) String.format(Locale.US, "$%.2f", run.costUsd) else null

        fun openRun(context: Context, runKey: String?): PendingIntent {
            val intent = Intent(context, MainActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
                runKey?.let { putExtra(EXTRA_RUN, it) }
            }
            return PendingIntent.getActivity(context, runKey?.hashCode() ?: 0, intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        }
    }
}
