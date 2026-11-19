package typle.server

import typle.lib.JsonWriter
import typle.lib.jsObject
import typle.lib.jsTrim

// The requests and answers of the handlers, apart from Ktor so that the tests call the handlers directly, and
// src/server/response.ts and src/server/request.ts.

/** A request: its method, its headers (names in lowercase, values as received) and its body. */
class Request(val method: String, val path: String, headers: List<Pair<String, String>>, private val readBody: () -> ByteArray) {
    private val headers: Map<String, List<String>> = headers.groupBy({ it.first.lowercase() }, { it.second })

    /** Headers.get of the Fetch API: the values joined with ", ", trimmed of HTTP white space; null when absent. */
    fun header(name: String): String? = headers[name.lowercase()]?.joinToString(", ") { it.trim(' ', '\t', '\r', '\n') }

    /** The body, read once; past maxBytes it is not read further (only its being too large is known). */
    fun body(): ByteArray = cached ?: readBody().also { cached = it }
    private var cached: ByteArray? = null

    fun asGet() = Request("GET", path, headers.flatMap { (k, vs) -> vs.map { k to it } }, readBody)
}

/** An answer: status, headers in order (a name may repeat) and the body, null for none. */
class Response(val status: Int, val headers: MutableList<Pair<String, String>>, val body: ByteArray?) {
    fun header(name: String): String? = headers.lastOrNull { it.first.equals(name, ignoreCase = true) }?.second
    fun text(): String = body?.toString(Charsets.UTF_8) ?: ""
}

object Answers {
    /** A JSON answer as reply() writes it: JSON.stringify plus a newline, never cached, varying by Cookie. */
    fun reply(status: Int, value: Any?): Response = Response(
        status,
        mutableListOf(
            "Content-Type" to "application/json; charset=utf-8",
            "Cache-Control" to "private, no-store",
            "Vary" to "Cookie",
        ),
        (JsonWriter.stringify(value) + "\n").toByteArray(Charsets.UTF_8),
    )

    fun refuse(status: Int, key: String) = reply(status, jsObject("error" to key))

    fun methodNotAllowed(allow: List<String>) = refuse(405, "methodNotAllowed").also { it.headers.add("Allow" to allow.joinToString(", ")) }

    fun withoutBody(response: Response) = Response(response.status, response.headers, null)
}

object RequestChecks {
    /** An app's request: it has an Authorization header that is not empty. */
    fun fromApp(request: Request) = !request.header("Authorization").isNullOrEmpty()

    fun fromAppOrOrigin(request: Request, origin: String) = fromApp(request) || request.header("Origin") == origin

    /** Whether the body is declared as application/json, compared as written. */
    fun jsonContent(request: Request) = (request.header("Content-Type") ?: "").split(';')[0].trim(' ', '\t', '\n', '\r', '\u000b', '\u000c', ' ', '﻿') == "application/json"
}
