package com.ryderdavid.nacre.data.chat

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class MockupsTest {
    @Test
    fun `media lines become mockups where they were and fenced code is left alone`() {
        val reply = "Here's the board.\n\nMEDIA:/Users/r/Library/Application Support/nacred/chats/ch_1/mockups/board-v2.html\n\nAnd the icon:\n`MEDIA:icon.png`\n```\nMEDIA:/tmp/x.html\n```"
        val parts = Mockups.split(reply)
        assertEquals(ReplyPart.Prose("Here's the board."), parts[0])
        assertEquals(ReplyPart.Mockup(MockupRef("board-v2.html")), parts[1])
        assertEquals(ReplyPart.Prose("And the icon:"), parts[2])
        assertEquals(ReplyPart.Mockup(MockupRef("icon.png")), parts[3])
        assertTrue((parts[4] as ReplyPart.Prose).text.contains("MEDIA:/tmp/x.html"))
        assertEquals(5, parts.size)
    }

    @Test
    fun `only a line of its own naming a servable file is a mockup`() {
        assertNull(Mockups.refOf("See MEDIA:board.html for it"))
        assertNull(Mockups.refOf("MEDIA:notes.txt"))
        assertNull(Mockups.refOf("MEDIA:/x/..secret.png"))
        assertEquals(MockupRef("a.svg"), Mockups.refOf("  MEDIA: /x/y/a.svg  "))
        assertEquals(listOf(ReplyPart.Prose("no mockups here")), Mockups.split("no mockups here"))
    }

    @Test
    fun `the page carries nacre's CSP in place of its own`() {
        val page = Mockups.injectCsp("<html><head><meta http-equiv=\"Content-Security-Policy\" content=\"default-src *\"><title>x</title></head><body></body></html>")
        assertTrue(page.contains("<head><meta http-equiv=\"Content-Security-Policy\" content=\"${Mockups.CSP}\"><title>"))
        assertTrue("default-src *" !in page)
        assertTrue(Mockups.injectCsp("<p>bare</p>").startsWith("<head><meta"))
        val svg = Mockups.page(MockupRef("d.svg"), "<svg/>".toByteArray())
        assertTrue(svg.contains("<body><svg/></body>") && svg.contains(Mockups.CSP))
    }
}
