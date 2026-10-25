package com.ryderdavid.nacre.data.model

import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.ui.bead.RunTarget
import com.ryderdavid.nacre.ui.runs.workingLabel
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** "Implement via Codex": when it is offered, what it sends, and how a Codex run reads. */
class CodexRunTest {
    private val json = BridgeClient.json

    @Test fun codexIsOfferedOnlyWhereTheComputerSaysItCanRunIt() {
        val ready = json.decodeFromString<Health>("""{"features":["claude","codex"],"codex_version":"0.159.2"}""")
        assertTrue(ready.runsCodex)
        assertNull(ready.codexBlocker)

        val signedOut = json.decodeFromString<Health>(
            """{"features":["claude"],"codex_unavailable":"Codex isn't signed in on this computer; run `codex login` there"}"""
        )
        assertFalse(signedOut.runsCodex)
        assertEquals("Codex isn't signed in on this computer; run `codex login` there", signedOut.codexBlocker)

        val older = json.decodeFromString<Health>("""{"features":["claude"]}""")
        assertEquals("This computer's nacred is too old to run Codex; update it", older.codexBlocker)
    }

    @Test fun anUnreachableComputerSaysSoInsteadOfItsLastHealth() {
        val repo = Repo(id = "r", name = "Nacre", path = "/x", hostName = "Mac Studio")
        assertEquals("Mac Studio isn't reachable right now", RunTarget(repo, online = false).codexBlocker)
        val ready = Health(features = listOf(Features.CODEX))
        assertNull(RunTarget(repo, online = true, health = ready).codexBlocker)
    }

    @Test fun aCodexRequestNamesTheAgentAndAClaudeOneSendsNothingNew() {
        assertEquals("""{"mode":"auto","agent":"codex"}""", json.encodeToString(RunRequest.serializer(), RunRequest(mode = "auto", agent = AGENT_CODEX)))
        assertEquals("""{"mode":"auto","model":"opus"}""", json.encodeToString(RunRequest.serializer(), RunRequest(mode = "auto", model = "opus")))
    }

    @Test fun aCodexRunIsLabelledCodex() {
        val r = json.decodeFromString<Run>("""{"id":"r_1","status":"running","agent":"codex","session_id":"th-1"}""")
        assertTrue(r.isCodex)
        assertTrue(r.isNacreWork)
        assertTrue(r.showsCancel)
        assertEquals("Codex is working…", workingLabel(r))
        assertEquals("Claude", Run(id = "r_2").agentName)
    }
}
