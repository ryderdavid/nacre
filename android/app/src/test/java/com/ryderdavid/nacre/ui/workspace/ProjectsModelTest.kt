package com.ryderdavid.nacre.ui.workspace

import com.ryderdavid.nacre.data.model.ProjectSummary
import com.ryderdavid.nacre.data.model.Repo
import com.ryderdavid.nacre.data.model.WorkSummary
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ProjectsModelTest {
    private val now = 1_000_000_000L
    private val studio = ComputerInfo("h1", "Mac Studio", online = true, lastSeenMs = now)
    private val air = ComputerInfo("h2", "MacBook Air", online = false, lastSeenMs = now - 12 * 60_000)
    private val repos = listOf(
        Repo("r1", "Nacre", "/n", hostId = "h1"),
        Repo("r2", "Beads", "/b", hostId = "h1"),
        Repo("r3", "dotfiles", "/d", hostId = "h2"),
    )

    private fun sections(filter: ProjectFilter = ProjectFilter.ALL, reported: Map<String, List<ProjectSummary>?> = emptyMap()) =
        ProjectsModel.sections(listOf(studio, air), repos, reported, emptyList(), emptyMap(), filter, "", now)

    @Test
    fun `projects are grouped by computer, and an offline one says when it was last checked`() {
        val s = sections()
        assertEquals(listOf("Mac Studio", "MacBook Air"), s.map { it.name })
        assertEquals("Connected", s[0].checked)
        assertEquals("Last checked 12 min ago", s[1].checked)
    }

    @Test
    fun `an offline computer's counts are unknown, never zero`() {
        val row = sections()[1].rows.single()
        assertNull(row.needsYou)
        assertEquals("Status unknown while offline", row.statusText)
    }

    @Test
    fun `a computer's own report wins over the fallback`() {
        val reported = mapOf("h1" to listOf(ProjectSummary(repo = "r1", name = "Nacre", status = "waiting", statusText = "Round stopped · 3 of 5 landed", needsYou = 2)))
        val nacre = sections(reported = reported)[0].rows.first()
        assertEquals("Nacre", nacre.name)
        assertEquals(2, nacre.needsYou)
        assertEquals("Round stopped · 3 of 5 landed", nacre.statusText)
    }

    @Test
    fun `the needs you filter keeps only projects waiting on Ryder`() {
        val reported = mapOf("h1" to listOf(ProjectSummary(repo = "r1", name = "Nacre", needsYou = 2), ProjectSummary(repo = "r2", name = "Beads")))
        val s = sections(ProjectFilter.NEEDS_YOU, reported)
        assertEquals(listOf("Nacre"), s.flatMap { it.rows }.map { it.name })
        assertEquals(2, ProjectsModel.needsYouTotal(s))
    }

    @Test
    fun `working filter uses running work`() {
        val reported = mapOf("h1" to listOf(ProjectSummary(repo = "r2", name = "Beads", status = "working", work = WorkSummary(state = "running"))))
        assertEquals(listOf("Beads"), sections(ProjectFilter.WORKING, reported).flatMap { it.rows }.map { it.name })
    }

    @Test
    fun `an offline project that last reported waiting stays in needs you, with its count unknown`() {
        val reported = mapOf("h2" to listOf(ProjectSummary(repo = "r3", name = "dotfiles", needsYou = 1)))
        val rows = sections(ProjectFilter.NEEDS_YOU, reported).flatMap { it.rows }
        assertEquals(listOf("dotfiles"), rows.map { it.name })
        assertNull(rows.single().needsYou)
    }
}
