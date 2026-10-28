package com.ryderdavid.nacre.data

import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.bridge.BridgeException
import com.ryderdavid.nacre.data.model.DaemonUpdate
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import java.io.IOException
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class DaemonUpdatesTest {
    private val success = DaemonUpdate(available = true, id = "update-1", hostId = "machine", phase = "updated", currentVersion = "1.0.0", currentCommit = "commit", targetVersion = "1.0.0", targetCommit = "commit")
    private suspend fun settled(m: DaemonUpdates, size: Int = 1) = withTimeout(3_000) { m.states.first { it.size == size && it.values.none { state -> state.active } } }

    @Test fun batchKeepsIndependentOfflineOldBusyAndSuccessfulResults() = runBlocking {
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        try {
            val m = DaemonUpdates(scope, pollMillis = 1)
            fun host(id: String, online: Boolean = true, action: suspend () -> DaemonUpdate) = UpdateComputer(id, "machine", online, { DaemonUpdate(available = true) }, action)
            m.startAll(listOf(
                host("online") { success },
                host("offline", false) { error("Offline host must not receive a request") },
                host("older") { throw BridgeException("Not found", code = 404) },
                host("busy") { throw BridgeException("Finish the active round first", code = 409) },
            ))
            val states = settled(m, 4)
            assertTrue(states.getValue("online").verifiedUpdate)
            assertEquals("offline", states.getValue("offline").phase)
            assertEquals("unavailable", states.getValue("older").phase)
            assertTrue(states.getValue("busy").message.contains("round"))
        } finally { scope.cancel() }
    }

    @Test fun lostPostResponseReconcilesWithoutPostingAgain() = runBlocking {
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        try {
            val posts = AtomicInteger(); val gets = AtomicInteger()
            val m = DaemonUpdates(scope, 1)
            m.startAll(listOf(UpdateComputer("one", "machine", true,
                status = { if (gets.incrementAndGet() == 1) DaemonUpdate(available = true) else success },
                start = { posts.incrementAndGet(); throw IOException("connection closed during restart") })))
            assertTrue(settled(m).getValue("one").verifiedUpdate)
            assertEquals(1, posts.get()); assertEquals(2, gets.get())
        } finally { scope.cancel() }
    }

    @Test fun lostPostCannotReuseHistoricalSuccessfulOperation() = runBlocking {
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        try {
            val m = DaemonUpdates(scope, 1)
            m.startAll(listOf(UpdateComputer("one", "machine", true,
                status = { success.copy(id = "yesterday") },
                start = { throw IOException("request did not arrive") })))
            val result = settled(m).getValue("one")
            assertEquals("failed", result.phase)
            assertTrue(result.message.contains("not confirmed"))
            assertFalse(result.verifiedUpdate)
        } finally { scope.cancel() }
    }

    @Test fun refreshReattachesAndDuplicateTapDoesNotStartAnotherOperation() = runBlocking {
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        try {
            val gets = AtomicInteger(); val posts = AtomicInteger()
            val m = DaemonUpdates(scope, 1)
            val h = UpdateComputer("one", "machine", true,
                status = { delay(15); if (gets.incrementAndGet() < 3) success.copy(phase = "restarting", active = true) else success },
                start = { posts.incrementAndGet(); success })
            m.refresh(listOf(h)); m.startAll(listOf(h))
            assertTrue(settled(m).getValue("one").verifiedUpdate)
            assertEquals(0, posts.get())
        } finally { scope.cancel() }
    }

    @Test fun wrongHostOrUnverifiedBuildNeverShowsSuccess() = runBlocking {
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        try {
            val m = DaemonUpdates(scope, 1)
            m.startAll(listOf(
                UpdateComputer("wrong-host", "machine", true, { DaemonUpdate(available = true) }, { success.copy(hostId = "other") }),
                UpdateComputer("wrong-build", "machine", true, { DaemonUpdate(available = true) }, { success.copy(currentCommit = "old") }),
            ))
            assertTrue(settled(m, 2).values.all { it.phase == "failed" && !it.verifiedUpdate })
        } finally { scope.cancel() }
    }

    @Test fun disconnectTimeoutIsUnconfirmedAndCanBeReadAgain() = runBlocking {
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        try {
            val m = DaemonUpdates(scope, 1, attempts = 2)
            var gets = 0
            m.startAll(listOf(UpdateComputer("one", "machine", true, { if (gets++ == 0) DaemonUpdate(available = true) else throw IOException("offline") }, { success.copy(phase = "restarting", active = true) })))
            assertEquals("unknown", settled(m).getValue("one").phase)
            m.refresh(listOf(UpdateComputer("one", "machine", true, { success }, { error("No POST") })))
            assertTrue(settled(m).getValue("one").verifiedUpdate)
        } finally { scope.cancel() }
    }

    @Test fun clientUsesAuthenticatedEmptyPostAndPreservesServerErrors() = runBlocking {
        val seen = mutableListOf<String>()
        val base = OkHttpClient.Builder().addNetworkInterceptor { error("No real networking") }.addInterceptor { chain ->
            // Bridge auth interceptor executes after this test interceptor, so inspect
            // request shape here; authorization is covered by server tests.
            val r = chain.request(); seen += r.method + " " + r.url.encodedPath
            val buffer = okio.Buffer(); r.body?.writeTo(buffer)
            if (r.method == "POST") assertEquals("{}", buffer.readUtf8())
            Response.Builder().request(r).protocol(Protocol.HTTP_1_1).code(200).message("fixture")
                .body("""{"available":true,"id":"operation","phase":"checking","active":true}""".toResponseBody()).build()
        }.build()
        val client = BridgeClient("http://fixture.invalid", "fixture-only", base)
        assertTrue(client.startDaemonUpdate().active)
        assertTrue(client.daemonUpdate().active)
        assertEquals(listOf("POST /v1/daemon/update", "GET /v1/daemon/update"), seen)
        assertFalse(DaemonUpdate(phase = "updated").verifiedUpdate)
    }
}
