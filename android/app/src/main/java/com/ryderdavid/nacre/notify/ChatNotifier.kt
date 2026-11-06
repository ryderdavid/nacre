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
 * "Chat needs your OK" on the round channel, one per request, opening the chat at its card; and one
 * notification per owner decision that newly waits, opening the chat that raised it, or the project's
 * queue at it for a Needs you item.
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
            .build()
        NotificationManagerCompat.from(context).notify(id, n)
    }

    override fun clearDecision(d: Decision) {
        NotificationManagerCompat.from(context).cancel(decisionNotificationId(d.key))
    }

    companion object {
        /** A decision notification: the project ("<host>~<repo>") and the decision's id. */
        const val EXTRA_DECISION_REPO = "nacre.decision_repo"
        const val EXTRA_DECISION = "nacre.decision_id"

        fun notificationId(chatKey: String, requestId: String) = "chat:$chatKey/$requestId".hashCode()
        fun decisionNotificationId(decisionKey: String) = "decision:$decisionKey".hashCode()
    }
}
