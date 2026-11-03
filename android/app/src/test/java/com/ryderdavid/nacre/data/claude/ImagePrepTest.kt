package com.ryderdavid.nacre.data.claude

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** Preparing a picked image: its type, its size and how it is brought under 3 MB. */
class ImagePrepTest {
    private fun bytes(vararg b: Int) = ByteArray(b.size) { b[it].toByte() }
    private fun ascii(s: String) = s.toByteArray(Charsets.ISO_8859_1)

    @Test fun sniffsTheFourSupportedTypesAndNothingElse() {
        assertEquals("image/jpeg", ImagePrep.sniff(bytes(0xff, 0xd8, 0xff, 0xe0)))
        assertEquals("image/png", ImagePrep.sniff(bytes(0x89) + ascii("PNG\r\n\u001a\n")))
        assertEquals("image/gif", ImagePrep.sniff(ascii("GIF89a....")))
        assertEquals("image/webp", ImagePrep.sniff(ascii("RIFF") + bytes(0, 0, 0, 0) + ascii("WEBPVP8 ")))
        assertNull(ImagePrep.sniff(ascii("%PDF-1.7")), "a PDF isn't attached")
        assertNull(ImagePrep.sniff(ascii("RIFF") + bytes(0, 0, 0, 0) + ascii("WAVE")), "nor is other RIFF")
        assertNull(ImagePrep.sniff(ByteArray(0)))
    }

    @Test fun downscalesTheLongEdgeTo1568() {
        assertEquals(1568 to 1176, ImagePrep.fit(4032, 3024))
        assertEquals(724 to 1568, ImagePrep.fit(1440, 3120))
        assertEquals(800 to 600, ImagePrep.fit(800, 600), "small images keep their size")
        assertEquals(4, ImagePrep.sampleSize(8000, 6000), "decodes at a size still above the target")
        assertEquals(1, ImagePrep.sampleSize(2000, 1000))
    }

    @Test fun keepsTheOriginalOnlyWhenItAlreadyFits() {
        assertTrue(ImagePrep.keepsOriginal("image/png", 1080, 1568, 2_000_000))
        assertFalse(ImagePrep.keepsOriginal("image/png", 1080, 2400, 2_000_000), "too long an edge")
        assertFalse(ImagePrep.keepsOriginal("image/jpeg", 1000, 1000, 3_000_001), "over 3 MB")
        assertFalse(ImagePrep.keepsOriginal("image/gif", 100, 100, 1000), "a GIF sends its first frame")
    }

    @Test fun reencodesAsJpegAtFallingQualityToStayWithin3MB() {
        val tried = mutableListOf<Pair<String, Int>>()
        val out = ImagePrep.shrink("image/png") { type, q ->
            tried += type to q
            ByteArray(if (type == "image/jpeg" && q <= 70) 2_500_000 else 4_000_000)
        }
        assertEquals("image/jpeg", out!!.first)
        assertEquals(2_500_000, out.second.size)
        assertEquals(listOf("image/png" to 90, "image/jpeg" to 90, "image/jpeg" to 80, "image/jpeg" to 70), tried)

        assertEquals("image/webp", ImagePrep.shrink("image/webp") { _, _ -> ByteArray(10) }!!.first, "its own type when that fits")
        assertEquals("image/png", ImagePrep.shrink("image/gif") { _, _ -> ByteArray(10) }!!.first, "a GIF's first frame as PNG")
        assertNull(ImagePrep.shrink("image/jpeg") { _, _ -> ByteArray(3_000_001) }, "can't be brought under 3 MB: skipped")
    }

    @Test fun renamesWhenTheTypeChanged() {
        assertEquals("Screenshot.jpg", ImagePrep.rename("Screenshot.png", "image/jpeg"))
        assertEquals("photo.jpeg", ImagePrep.rename("photo.jpeg", "image/jpeg"))
        assertEquals("anim.png", ImagePrep.rename("anim.gif", "image/png"))
        assertEquals("image.png", ImagePrep.rename("image", "image/png"))
    }

    @Test fun skippedFilesAreNamedInTheSnackbar() {
        assertTrue("notes.pdf" in SkipReasons.unsupported("notes.pdf"))
        assertTrue("broken.png" in SkipReasons.unreadable("broken.png"))
        assertTrue("huge.png" in SkipReasons.tooLarge("huge.png"))
    }
}
