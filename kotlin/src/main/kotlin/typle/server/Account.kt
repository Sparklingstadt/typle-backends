package typle.server

import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneOffset

// src/server/account.ts: who a request is signed in as, the Better Auth session its cookie names.

/** Reads the user of a Better Auth session by its token, while the session has not expired at now; null otherwise. */
fun interface SessionSource {
    fun sessionUser(token: String, now: Instant): Session?
}

object Account {
    /**
     * The shared demo account's ID (src/server/demoLogin.ts). Its sessions stop working when its login is turned off
     * (DEMO_LOGIN), not only new logins.
     */
    const val DEMO_ACCOUNT_ID = "demo:shared"

    /**
     * The value of the first cookie named name in the Cookie header, as better-call's parseCookies reads it: the first
     * pair with that name wins, and a value that starts with a double quote loses its first and last character. The
     * value is not decoded here (Sessions.token does).
     */
    fun cookieValue(request: Request, name: String): String? {
        for (part in (request.header("Cookie") ?: "").split(';')) {
            val at = part.indexOf('=')
            if (at < 0 || part.substring(0, at).trim(::isJsWhite) != name) continue
            val value = part.substring(at + 1).trim(::isJsWhite)
            return if (value.startsWith('"')) value.substring(1, maxOf(1, value.length - 1)) else value
        }
        return null
    }

    private fun isJsWhite(c: Char) = typle.lib.isJsSpace(c)

    /**
     * The sessions are Better Auth's rows (typle_auth_sessions, migration 0042 of typle-r), read as Better Auth's
     * findSession and getSession read them: the session's row and its user's, only while the session has not expired
     * (expires_at not before now). Signing out of every device deletes the user's rows.
     */
    fun databaseSessions(db: Database) = SessionSource { token, now ->
        db.withConnection { c ->
            c.query(
                """SELECT u.id, u.name, u.image FROM typle_auth_sessions s
        JOIN typle_auth_users u ON u.id = s.user_id WHERE s.token = ? AND s.expires_at >= ?""",
                token, OffsetDateTime.ofInstant(now, ZoneOffset.UTC),
            ) { rows -> if (rows.next()) Session(rows.getString(1), rows.getString(2), rows.getString(3) ?: "") else null }
        }
    }

    sealed interface SignedIn
    class User(val user: Session) : SignedIn
    class Refusal(val response: Response) : SignedIn

    /**
     * signedIn of src/server/account.ts, for a request that reads or changes the account's data: loginRequired without
     * a session cookie, with one whose signature does not check out, or with one of no session or an expired one;
     * databaseUnavailable when the session cannot be read (also without BETTER_AUTH_SECRET, as Better Auth then fails to
     * start in typle-r), as the data cannot be either. No cookie is removed: typle-r reads the session through
     * auth.api.getSession, whose cookies do not reach the answer. The session row is always read (typle-r's cookie cache
     * may answer for up to 5 minutes without it), so a session deleted by signing out of every device is refused at once.
     */
    fun signedIn(request: Request, sessions: SessionSource?, settings: LoginSettings, now: Instant = Instant.now()): SignedIn {
        if (settings.secret.isEmpty()) {
            Report.failure(request, IllegalStateException("BETTER_AUTH_SECRET is not set"), "session.check")
            return Refusal(Answers.refuse(503, "databaseUnavailable"))
        }
        val token = cookieValue(request, settings.cookieName)?.let { Sessions.token(it, settings.secret) }
            ?: return Refusal(Answers.refuse(401, "loginRequired"))
        val user = try {
            if (sessions == null) throw IllegalStateException("no database to read the session from")
            sessions.sessionUser(token, now)
        } catch (error: Exception) {
            Report.failure(request, error, "session.check")
            return Refusal(Answers.refuse(503, "databaseUnavailable"))
        }
        if (user == null || (user.sub == DEMO_ACCOUNT_ID && !settings.demoLogin)) return Refusal(Answers.refuse(401, "loginRequired"))
        return User(user)
    }
}

/** src/server/report.ts: a failure answered with a 503, in the log, as the Next.js routes do. */
object Report {
    fun failure(request: Request, error: Throwable, operation: String) {
        System.err.println("${request.method} ${request.path}: $operation: $error")
    }
}
