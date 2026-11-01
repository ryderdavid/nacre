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
        if (savedInstanceState == null) deepLink = parse(intent)
        val container = (application as NacreApp).container
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
        intent.getStringExtra(RunNotifier.EXTRA_RUN)?.let { return DeepLink.OpenRun(it) }
        intent.getStringExtra(RunNotifier.EXTRA_ROUND)?.let { return DeepLink.OpenRound(it) }
        intent.getStringExtra(RunNotifier.EXTRA_CHANGE_BEAD)?.let { bead ->
            intent.getStringExtra(RunNotifier.EXTRA_CHANGE_REPO)?.let { return DeepLink.OpenChange(it, bead) }
        }
        return intent.dataString?.let(PairRequest::parse)?.let(DeepLink::Pair)
    }
}
