package typle.server

import typle.lib.jsObject

// The routes of src/app/api/{health,workspace,workspace/records}/route.ts, behind the comparison's gate, with the headers
// next.config.mjs adds to every answer and the x-typle-backend header that says this backend answered.

class Backend(
    private val compare: Boolean,
    private val open: () -> Database?,
    private val settings: () -> LoginSettings?,
) {
    companion object {
        const val BACKEND_HEADER = "x-typle-backend"

        /** The headers next.config.mjs applies to every path (and to /api paths, the CSP), in its order. */
        private val securityHeaders = listOf(
            "X-Frame-Options" to "DENY",
            "X-Content-Type-Options" to "nosniff",
            "Referrer-Policy" to "strict-origin-when-cross-origin",
            "Permissions-Policy" to "camera=(), microphone=(), geolocation=()",
            "Content-Security-Policy" to "frame-ancestors 'none'; base-uri 'self'; object-src 'none'",
        )

        fun fromEnv(env: Map<String, String>): Backend {
            val database by lazy { Database.fromEnv(env) }
            val login = LoginSettings.read(env)
            return Backend(env["TYPLE_BACKEND_COMPARE"] == "1", { database }, { login })
        }
    }

    /** The answer to request, with the headers every answer carries. */
    fun handle(request: Request): Response {
        val response = route(request)
        response.headers.addAll(securityHeaders)
        response.headers.add(BACKEND_HEADER to "kotlin")
        return response
    }

    private fun route(request: Request): Response {
        // Outside the comparison nothing is served, so a deployment without TYPLE_BACKEND_COMPARE=1 reads and writes nothing.
        if (!compare) return notFound()
        return when (request.path) {
            "/api/health" -> when (request.method) {
                "GET" -> checkHealth(request)
                "HEAD" -> Answers.withoutBody(checkHealth(request))
                else -> Answers.methodNotAllowed(listOf("GET", "HEAD"))
            }
            "/api/workspace" -> when (request.method) {
                "GET", "PUT" -> Workspace.answerWorkspace(request, open, settings)
                "HEAD" -> Answers.withoutBody(Workspace.answerWorkspace(request.asGet(), open, settings))
                else -> Answers.methodNotAllowed(listOf("GET", "HEAD", "PUT"))
            }
            "/api/workspace/records" -> when (request.method) {
                "POST", "DELETE" -> Workspace.answerRecords(request, open, settings)
                else -> Answers.methodNotAllowed(listOf("POST", "DELETE"))
            }
            else -> notFound()
        }
    }

    private fun notFound() = Answers.reply(404, jsObject("error" to "notFound"))

    /** src/server/health.ts: 200 once the database answers within 5 seconds, 503 databaseUnavailable otherwise. */
    private fun checkHealth(request: Request): Response {
        try {
            val db = open() ?: throw IllegalStateException("DATABASE_URL is not set")
            db.ping()
        } catch (error: Throwable) {
            System.err.println("GET /api/health: $error")
            return Answers.refuse(503, "databaseUnavailable")
        }
        return Answers.reply(200, jsObject("status" to "ok"))
    }
}
