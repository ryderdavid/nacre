package com.ryderdavid.nacre.notify

import android.Manifest
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import com.ryderdavid.nacre.MainActivity
import com.ryderdavid.nacre.R
import com.ryderdavid.nacre.data.DecisionAlerts
import com.ryderdavid.nacre.data.DecisionNotices
import com.ryderdavid.nacre.data.chat.ChatNotices
import com.ryderdavid.nacre.data.chat.PermissionCard
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.model.Keys

/** The title of a permission request's notification. */
const val CHAT_NEEDS_OK = "Chat needs your OK"

/**
 * "Chat needs your OK" on the round channel, one per request, opening the chat at its card; and the owner
 * decisions that newly wait, grouped under one summary, "N things need you". The summary opens the
 * project with the most waiting and starts the Needs you walk-through there; each decision under it
 * opens the chat that raised it, or starts the walk-through at it.
 */
class ChatNotifier(private val context: Context) : ChatNotices, DecisionNotices {
    override fun post(chat: ChatRecord, card: PermissionCard) {
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) return
        val body = listOf(card.step, card.reason).filter { it.isNotBlank() }.joinToString("\n")
        val intent = Intent(context, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
            putExtra(RunNotifier.EXTRA_CHAT, chat.key)
            putExtra(RunNotifier.EXTRA_CHAT_REQUEST, card.requestId)
        }
        val id = notificationId(chat.key, card.requestId)
        val n = NotificationCompat.Builder(context, RunNotifier.CH_ROUNDS)
            .setSmallIcon(R.drawable.ic_stat_nacre)
            .setContentTitle(CHAT_NEEDS_OK)
            .setContentText(card.step)
            .setStyle(NotificationCompat.BigTextStyle().bigText(body))
            .setSubText(listOfNotNull(chat.hostName.ifBlank { null }, chat.title.ifBlank { null }).joinToString(" · "))
            .setContentIntent(PendingIntent.getActivity(context, id, intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT))
            .setAutoCancel(true)
            .setCategory(NotificationCompat.CATEGORY_STATUS)
            .build()
        NotificationManagerCompat.from(context).notify(id, n)
    }

    override fun clear(chatKey: String, requestId: String) {
        NotificationManagerCompat.from(context).cancel(notificationId(chatKey, requestId))
    }

    override fun postDecision(d: Decision) {
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) return
        val (title, text) = DecisionAlerts.text(d)
        val intent = Intent(context, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
            putExtra(EXTRA_DECISION_REPO, Keys.of(d.hostId, d.repo))
            putExtra(EXTRA_DECISION, d.id)
            d.chatId?.takeIf { it.isNotBlank() }?.let { putExtra(RunNotifier.EXTRA_CHAT, Keys.of(d.hostId, it)) }
        }
        val id = decisionNotificationId(d.key)
        val body = listOf(text, d.consequences.joinToString("\n")).filter { it.isNotBlank() }.distinct().joinToString("\n")
        val n = NotificationCompat.Builder(context, RunNotifier.CH_ROUNDS)
            .setSmallIcon(R.drawable.ic_stat_nacre)
            .setContentTitle(title)
            .setContentText(text)
            .setStyle(NotificationCompat.BigTextStyle().bigText(body))
            .setSubText(d.repoName.ifBlank { null })
            .setContentIntent(PendingIntent.getActivity(context, id, intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT))
            .setAutoCancel(true)
            .setCategory(NotificationCompat.CATEGORY_STATUS)
            .setGroup(GROUP_NEEDS_YOU)
            .build()
        NotificationManagerCompat.from(context).notify(id, n)
        synchronized(waiting) { waiting[d.key] = Keys.of(d.hostId, d.repo) to d.repoName.ifBlank { d.repo } }
        postSummary()
    }

    override fun clearDecision(d: Decision) {
        NotificationManagerCompat.from(context).cancel(decisionNotificationId(d.key))
        synchronized(waiting) { waiting.remove(d.key) }
        postSummary()
    }

    /** The one "N things need you" notification over the decisions posted, or none when nothing is left. */
    private fun postSummary() {
        val nm = NotificationManagerCompat.from(context)
        val byProject = synchronized(waiting) { waiting.values.groupingBy { it }.eachCount() }
        val total = byProject.values.sum()
        if (total == 0) { nm.cancel(SUMMARY_ID); return }
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) return
        val (busiest, _) = byProject.maxBy { it.value }.key
        val intent = Intent(context, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
            // A project and no decision: the walk-through starts at the top of its queue.
            putExtra(EXTRA_DECISION_REPO, busiest)
        }
        val title = summaryTitle(total)
        val style = NotificationCompat.InboxStyle().setBigContentTitle(title)
        byProject.entries.sortedByDescending { it.value }.forEach { (p, n) -> style.addLine("${p.second} · $n") }
        val n = NotificationCompat.Builder(context, RunNotifier.CH_ROUNDS)
            .setSmallIcon(R.drawable.ic_stat_nacre)
            .setContentTitle(title)
            .setContentText("Tap to go through them in Chat")
            .setStyle(style)
            .setGroup(GROUP_NEEDS_YOU)
            .setGroupSummary(true)
            .setOnlyAlertOnce(true)
            .setContentIntent(PendingIntent.getActivity(context, SUMMARY_ID, intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT))
            .setAutoCancel(true)
            .setCategory(NotificationCompat.CATEGORY_STATUS)
            .build()
        nm.notify(SUMMARY_ID, n)
    }

    companion object {
        /** A decision notification: the project ("<host>~<repo>") and the decision's id. */
        const val EXTRA_DECISION_REPO = "nacre.decision_repo"
        const val EXTRA_DECISION = "nacre.decision_id"

        fun notificationId(chatKey: String, requestId: String) = "chat:$chatKey/$requestId".hashCode()

        /** Decision notifications share this group under one summary. */
        const val GROUP_NEEDS_YOU = "nacre.needs_you"
        private val SUMMARY_ID = "needs-you-summary".hashCode()

        /** The decisions posted now, by decision key: their project key and name. Shared by every notifier. */
        private val waiting = mutableMapOf<String, Pair<String, String>>()

        fun summaryTitle(total: Int) = if (total == 1) "1 thing needs you" else "$total things need you"
        fun decisionNotificationId(decisionKey: String) = "decision:$decisionKey".hashCode()
    }
}
