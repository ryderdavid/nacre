package com.ryderdavid.nacre.data

import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.Repo
import kotlin.test.Test
import kotlin.test.assertEquals

class GroupReposTest {
    private fun repo(host: String, id: String, remote: String = "") =
        Repo(id = id, name = id, path = "/x/$id", gitRemote = remote, hostId = host, hostName = host)

    private val studio = "c0"
    private val laptop = "c1"

    @Test fun sameRemoteOnTwoComputersIsOneGroup() {
        val groups = groupRepos(
            listOf(repo(laptop, "b1", "github.com/me/app"), repo(studio, "a1", "github.com/me/app"), repo(studio, "a2")),
            hostOrder = listOf(studio, laptop),
            homeHosts = emptyMap(),
        )
        assertEquals(2, groups.size)
        val shared = groups.first { it.isShared }
        assertEquals(listOf(studio, laptop), shared.members.map { it.hostId }, "members follow pairing order")
        assertEquals(studio, shared.home.hostId, "first-paired computer is home by default")
    }

    @Test fun chosenHomeWins() {
        val g = groupRepos(
            listOf(repo(studio, "a1", "github.com/me/app"), repo(laptop, "b1", "github.com/me/app")),
            hostOrder = listOf(studio, laptop),
            homeHosts = mapOf("git:github.com/me/app" to laptop),
        ).single()
        assertEquals(Keys.of(laptop, "b1"), g.home.key)
    }

    @Test fun reposWithoutRemoteNeverMerge() {
        val groups = groupRepos(listOf(repo(studio, "a"), repo(laptop, "a")), listOf(studio, laptop), emptyMap())
        assertEquals(2, groups.size, "same local id on two computers is still two repos")
    }

    @Test fun keysRoundTrip() {
        val k = Keys.of("c1", "3f9a1c0b2d4e")
        assertEquals("c1", Keys.host(k))
        assertEquals("3f9a1c0b2d4e", Keys.id(k))
    }

    @Test fun oneCheckoutPerComputer() {
        val g = groupRepos(
            listOf(
                Repo(id = "wt", name = "app", path = "/x/app-feature", gitRemote = "github.com/me/app", hostId = studio),
                Repo(id = "main", name = "app", path = "/x/app", gitRemote = "github.com/me/app", hostId = studio),
            ),
            listOf(studio), emptyMap(),
        ).single()
        assertEquals(listOf("main"), g.members.map { it.id })
    }
}
