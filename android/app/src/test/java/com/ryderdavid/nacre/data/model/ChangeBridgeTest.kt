package com.ryderdavid.nacre.data.model

import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.bridge.BridgeException
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class ChangeBridgeTest {
    private fun client(code: Int, body: String): BridgeClient = BridgeClient("http://fixture.invalid", "fixture-only", OkHttpClient.Builder().addInterceptor { chain ->
        assertEquals("/v1/repos/repo/issues/NAC-1/change", chain.request().url.encodedPath)
        Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(code).message("Fixture").body(body.toResponseBody()).build()
    }.build())

    @Test fun olderBridgeIsUnavailableWithoutBreakingTheBead() = runBlocking {
        val result = client(404, "404 page not found").change("repo", "NAC-1")
        assertFalse(result.available)
        assertTrue(result.reason.contains("Update nacred"))
    }

    @Test fun authenticationFailureDoesNotMasqueradeAsUnsupported() = runBlocking {
        val failure = assertFailsWith<BridgeException> { client(401, "").change("repo", "NAC-1") }
        assertEquals(401, failure.code)
    }

    @Test fun coveredDispositionDoesNotNeedArtifactsOrAnEpic() = runBlocking {
        val result = client(200, """{"available":true,"change":{"id":"sf-1","seed_id":"NAC-1","phase":"covered","route":"covered","summary":"Already covered","references":["openspec/specs/drafts/spec.md"]}}""").change("repo", "NAC-1")
        assertTrue(result.available)
        assertTrue(result.change!!.implementationIds.isEmpty())
        assertEquals("", result.change.epicId)
    }
}
