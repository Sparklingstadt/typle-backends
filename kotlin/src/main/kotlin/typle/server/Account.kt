package typle.server

import java.time.OffsetDateTime

// src/server/account.ts: who a request is signed in as, and whether that login still counts (typle_session_revocations).

object Account {
    /** The value of the cookie name, the first one when there are several; quotes around it are dropped. */
    fun cookieValue(request: Request, name: String): String? {
        for (part in (request.header("Cookie") ?: "").split(';')) {
            val at = part.indexOf('=')
            if (at < 0 || part.substring(0, at).trim(::isJsWhite) != name) continue
            val value = part.substring(at + 1).trim(::isJsWhite)
            return if (value.length >= 2 && value.startsWith('"') && value.endsWith('"')) value.substring(1, value.length - 1) else value
        }
        return null
    }

    private fun isJsWhite(c: Char) = typle.lib.isJsSpace(c)

    private val tokenSpace = Regex("[ \t]")

    /** The signed-in user: from an app's bearer token when there is an Authorization header, otherwise from the cookie. */
    fun currentUser(request: Request, settings: LoginSettings, now: Double = System.currentTimeMillis() / 1000.0): Session? {
        val authorization = request.header("Authorization")
        if (!authorization.isNullOrEmpty()) {
            val token = if (authorization.startsWith("Bearer ")) authorization.substring("Bearer ".length) else ""
            if (token.isEmpty() || tokenSpace.containsMatchIn(token)) return null
            return Sessions.open(settings.appKey, token, now)
        }
        val cookie = cookieValue(request, settings.cookieName) ?: return null
        return Sessions.open(settings.sessionKey, cookie, now)
    }

    /** When the owner last signed out of every device, in milliseconds, or null. */
    fun readRevokedAt(db: Database, owner: String): Long? = db.withConnection { c ->
        c.query("SELECT revoked_at FROM typle_session_revocations WHERE owner_id = ?", owner) { rows ->
            if (rows.next()) rows.getObject(1, OffsetDateTime::class.java).toInstant().toEpochMilli() else null
        }
    }

    fun revokedBy(user: Session, revokedAt: Long?) = revokedAt != null && user.issuedAt * 1000 <= revokedAt

    fun dropCookie(response: Response, settings: LoginSettings): Response {
        val secure = if (settings.secure) "; Secure" else ""
        response.headers.add("Set-Cookie" to "${settings.cookieName}=; Path=/; Max-Age=0; HttpOnly$secure; SameSite=Lax")
        return response
    }

    sealed interface SignedIn
    class User(val user: Session) : SignedIn
    class Refusal(val response: Response) : SignedIn

    /** The user of a request that reads or changes the account's data, or the answer to refuse it with. */
    fun signedIn(request: Request, db: Database?, settings: LoginSettings): SignedIn {
        val user = currentUser(request, settings) ?: return Refusal(Answers.refuse(401, "loginRequired"))
        val revokedAt = try {
            if (db == null) throw IllegalStateException("no database to check the session against")
            readRevokedAt(db, user.sub)
        } catch (error: Exception) {
            Report.failure(request, error, "session.check")
            return Refusal(Answers.refuse(503, "databaseUnavailable"))
        }
        if (revokedBy(user, revokedAt)) {
            val refusal = Answers.refuse(401, "loginRequired")
            return Refusal(if (!request.header("Authorization").isNullOrEmpty()) refusal else dropCookie(refusal, settings))
        }
        return User(user)
    }
}

/** src/server/report.ts: a failure answered with a 503, in the log, as the Next.js routes do. */
object Report {
    fun failure(request: Request, error: Throwable, operation: String) {
        System.err.println("${request.method} ${request.path}: $operation: $error")
    }
}
