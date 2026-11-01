package com.ryderdavid.nacre.data.model

import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.ui.runs.LIVE_SEND_NOTE
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/** "Send to live session" (docs/API.md, Live sends): offered only where the bridge says so. */
class LiveSendTest {
    private fun decode(json: String) = BridgeClient.json.decodeFromString<Run>(json)

    private val adopted = """"origin":"adopted","agent":"claude-code","activity":"working","can_cancel":false,"can_followup":false,"session_id":"s1""""

    @Test fun offeredOnlyOnALiveAdoptedRunThatPassesTheGuard() {
        val ok = decode("""{"id":"r_1","status":"running",$adopted,"can_send_live":true}""")
        assertTrue(ok.showsSendLive)
        assertFalse(ok.showsFollowUp, "the ordinary follow-up stays refused")

        val guarded = decode("""{"id":"r_2","status":"running",$adopted,"live_send_unavailable":"unsupported peer protocol 2 (nacred speaks 1)"}""")
        assertFalse(guarded.showsSendLive)
        assertEquals("unsupported peer protocol 2 (nacred speaks 1)", guarded.liveSendUnavailable)

        assertFalse(ok.copy(status = "succeeded", activity = ACTIVITY_ENDED).showsSendLive, "an ended session takes nothing")
        assertFalse(Run(id = "r_3", status = "running", canSendLive = true).showsSendLive, "only adopted runs")
        assertFalse(decode("""{"id":"r_4","status":"running","session_id":"s"}""").showsSendLive, "an older bridge never offers it")
    }

    @Test fun receiptsReadAsTheyShould() {
        val r = decode(
            """{"id":"r_1","status":"running",$adopted,"can_send_live":true,"peer_messages":[
                {"msg_id":"a","text":"one","sent_at":"2026-10-27T10:00:00Z","status":"held","reason":"held for approval"},
                {"msg_id":"b","text":"two","sent_at":"2026-10-27T10:01:00Z","status":"refused"},
                {"msg_id":"c","text":"three","sent_at":"2026-10-27T10:02:00Z","status":"dropped","reason":"shutdown"},
                {"msg_id":"d","text":"four","sent_at":"2026-10-27T10:03:00Z","status":"delivered","confirmed":true,"confirmed_at":"2026-10-27T10:03:02Z"},
                {"msg_id":"e","text":"five","sent_at":"2026-10-27T10:04:00Z","status":"unconfirmed"}]}"""
        )
        val (held, refused, dropped, delivered, unconfirmed) = r.peerMessages
        assertEquals("Held · waiting for approval on the Mac", held.label)
        assertFalse(held.failed)
        assertTrue(refused.failed); assertTrue(refused.label.startsWith("Refused"))
        assertTrue(dropped.failed); assertEquals("shutdown", dropped.reason)
        assertEquals("Dropped by the session", dropped.label)
        assertFalse(delivered.failed); assertEquals("Delivered", delivered.label)
        assertTrue(unconfirmed.failed)
        assertFalse(unconfirmed.copy(confirmed = true).failed, "a late transcript entry settles it")
    }

    @Test fun theNoteSaysWhatALiveSendCannotDo() {
        assertTrue("peer message" in LIVE_SEND_NOTE)
        assertTrue("can't approve" in LIVE_SEND_NOTE)
        assertTrue("slash commands" in LIVE_SEND_NOTE)
    }
}
