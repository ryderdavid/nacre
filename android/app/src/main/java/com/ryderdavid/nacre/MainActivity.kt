package com.ryderdavid.nacre

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.core.splashscreen.SplashScreen.Companion.installSplashScreen
import com.ryderdavid.nacre.notify.ChatNotifier
import com.ryderdavid.nacre.notify.RunNotifier
import com.ryderdavid.nacre.ui.DeepLink
import com.ryderdavid.nacre.ui.NacreRoot
import com.ryderdavid.nacre.ui.settings.PairRequest

class MainActivity : ComponentActivity() {
    private var deepLink by mutableStateOf<DeepLink?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        installSplashScreen()
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        val container = (application as NacreApp).container
        if (savedInstanceState == null) deepLink = parse(intent)
        setContent {
            NacreRoot(container, deepLink, onDeepLinkHandled = { deepLink = null })
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        parse(intent)?.let { deepLink = it }
    }

    private fun parse(intent: Intent?): DeepLink? {
        intent ?: return null
        intent.getStringExtra(ChatNotifier.EXTRA_DECISION_REPO)?.let { repoKey -> return decisionLink(repoKey, intent) }
        intent.getStringExtra(RunNotifier.EXTRA_CHAT)?.let { return DeepLink.OpenChat(it, intent.getStringExtra(RunNotifier.EXTRA_CHAT_REQUEST)) }
        intent.getStringExtra(RunNotifier.EXTRA_RUN)?.let { return DeepLink.OpenRun(it) }
        intent.getStringExtra(RunNotifier.EXTRA_ROUND)?.let { return DeepLink.OpenRound(it) }
        intent.getStringExtra(RunNotifier.EXTRA_CHANGE_BEAD)?.let { bead ->
            intent.getStringExtra(RunNotifier.EXTRA_CHANGE_REPO)?.let { return DeepLink.OpenChange(it, bead) }
        }
        return intent.dataString?.let(PairRequest::parse)?.let(DeepLink::Pair)
    }

    /**
     * A decision notification. One a chat raised opens that chat, where its card is; a Needs you item
     * opens the project's queue at it: the queue is pointed at the decision and asked to open, and the
     * project's workspace is shown through the change link (the shell has no queue link yet).
     */
    private fun decisionLink(repoKey: String, intent: Intent): DeepLink {
        val id = intent.getStringExtra(ChatNotifier.EXTRA_DECISION)
        val chat = intent.getStringExtra(RunNotifier.EXTRA_CHAT)
        (application as NacreApp).container.repository.focusQueue(repoKey, id, open = chat == null)
        return DeepLink.OpenDecision(repoKey, id, chat)
    }
}
