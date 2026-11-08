package com.ryderdavid.nacre.data.chat

import kotlinx.coroutines.CancellationException

/**
 * A mockup the agent showed: a file it wrote into the chat's mockups directory and named on a line of its
 * reply as `MEDIA:<path>`. nacred serves only that directory, by file name.
 */
data class MockupRef(val name: String) {
    val ext get() = name.substringAfterLast('.', "").lowercase()
    val isHtml get() = ext == "html" || ext == "htm"
    val isSvg get() = ext == "svg"
}

/** A reply in pieces: prose, and mockups where their `MEDIA:` lines were. */
sealed interface ReplyPart {
    data class Prose(val text: String) : ReplyPart
    data class Mockup(val ref: MockupRef) : ReplyPart
}

object Mockups {
    val EXTS = setOf("html", "htm", "png", "jpg", "jpeg", "webp", "gif", "svg")
    // The path may have spaces (nacred's directory is under "Application Support"); the file name may not.
    private val line = Regex("""^\s*`?MEDIA:\s*([^`]+?)`?\s*$""")
    private val name = Regex("""^[A-Za-z0-9][A-Za-z0-9._-]{0,120}$""")

    /** The mockup a line names, or null: a `MEDIA:` line alone, a file name nacred serves, a mockup's extension. */
    fun refOf(text: String): MockupRef? {
        val path = line.matchEntire(text)?.groupValues?.get(1) ?: return null
        val file = path.trim().trim('"', '\'').substringAfterLast('/')
        if (!name.matches(file) || ".." in file) return null
        return MockupRef(file).takeIf { it.ext in EXTS }
    }

    /** Splits a reply at its `MEDIA:` lines, leaving fenced code alone. A reply with none is one prose part. */
    fun split(text: String): List<ReplyPart> {
        if ("MEDIA:" !in text) return listOf(ReplyPart.Prose(text))
        val out = mutableListOf<ReplyPart>()
        val prose = StringBuilder()
        var fenced = false
        fun flush() {
            val t = prose.toString().trim('\n')
            if (t.isNotBlank()) out += ReplyPart.Prose(t)
            prose.clear()
        }
        for (l in text.split('\n')) {
            if (l.trimStart().startsWith("```")) fenced = !fenced
            val ref = if (fenced) null else refOf(l)
            if (ref != null) { flush(); out += ReplyPart.Mockup(ref) } else prose.append(l).append('\n')
        }
        flush()
        return out
    }

    /**
     * The page a mockup loads: its HTML with a Content-Security-Policy that lets inline and https scripts,
     * styles, images and fonts load and nothing else (no forms, no frames, no plugins); an SVG wrapped in a
     * page that fits it to the width.
     */
    fun page(ref: MockupRef, bytes: ByteArray): String {
        val body = bytes.toString(Charsets.UTF_8)
        val html = if (ref.isSvg) "<!doctype html><html><head><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">" +
            "<style>html,body{margin:0;background:transparent}svg{width:100%;height:auto;display:block}</style></head><body>$body</body></html>"
        else body
        return injectCsp(html)
    }

    const val CSP = "default-src 'none'; script-src 'unsafe-inline' https:; style-src 'unsafe-inline' https:; " +
        "img-src data: blob: https:; font-src data: https:; connect-src https:; form-action 'none'; base-uri 'none'; " +
        "object-src 'none'; frame-src 'none'"

    private val existingCsp = Regex("""<meta[^>]+http-equiv\s*=\s*["']?content-security-policy["']?[^>]*>""", RegexOption.IGNORE_CASE)
    private val head = Regex("""<head[^>]*>""", RegexOption.IGNORE_CASE)
    private val htmlTag = Regex("""<html[^>]*>""", RegexOption.IGNORE_CASE)

    /** [html] with any CSP meta of its own removed and [CSP] put first in its head. */
    fun injectCsp(html: String): String {
        val meta = "<meta http-equiv=\"Content-Security-Policy\" content=\"$CSP\">"
        val clean = existingCsp.replace(html, "")
        head.find(clean)?.let { return clean.substring(0, it.range.last + 1) + meta + clean.substring(it.range.last + 1) }
        htmlTag.find(clean)?.let { return clean.substring(0, it.range.last + 1) + "<head>$meta</head>" + clean.substring(it.range.last + 1) }
        return "<head>$meta</head>$clean"
    }
}

/** Mockup bytes fetched from nacred, the most recent [max] kept in memory. A failed fetch isn't kept. */
class MockupCache(private val link: ChatLink, private val max: Int = 12) {
    private val cache = object : LinkedHashMap<String, ByteArray>(16, 0.75f, true) {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<String, ByteArray>?) = size > max
    }

    suspend fun bytes(chatKey: String, ref: MockupRef): Result<ByteArray> {
        val id = "$chatKey/${ref.name}"
        synchronized(cache) { cache[id] }?.let { return Result.success(it) }
        return try {
            val b = link.chatMockup(chatKey, ref.name)
            synchronized(cache) { cache[id] = b }
            Result.success(b)
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            Result.failure(e)
        }
    }
}
