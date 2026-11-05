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
import com.ryderdavid.nacre.data.chat.ChatNotices
import com.ryderdavid.nacre.data.chat.PermissionCard
import com.ryderdavid.nacre.data.model.ChatRecord

/** The title of a permission request's notification. */
const val CHAT_NEEDS_OK = "Chat needs your OK"

/** "Chat needs your OK" on the round channel, one per request, opening the chat at its card. */
class ChatNotifier(private val context: Context) : ChatNotices {
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

    companion object {
        fun notificationId(chatKey: String, requestId: String) = "chat:$chatKey/$requestId".hashCode()
    }
}
