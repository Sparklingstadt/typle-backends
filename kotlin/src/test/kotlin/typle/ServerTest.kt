package typle

import org.junit.jupiter.api.AfterAll
import org.junit.jupiter.api.Assumptions.assumeTrue
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.TestFactory
import org.junit.jupiter.api.TestInstance
import typle.lib.Interests
import typle.lib.JsArray
import typle.lib.JsObject
import typle.lib.JsonParser
import typle.lib.JsonWriter
import typle.lib.AppError
import typle.server.Backend
import typle.server.Conflict
import typle.server.Database
import typle.server.DatabaseUrl
import typle.server.LoginSettings
import typle.server.Account
import typle.server.Session
import typle.server.SessionSource
import typle.server.Sessions
import typle.server.Request
import typle.server.Response
import typle.server.Workspace
import typle.server.execute
import typle.server.query
import java.time.Duration
import java.time.Instant
import java.util.concurrent.Callable
import java.util.concurrent.Executors
import kotlin.random.Random
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** The routes without a database: the gate, the headers every answer carries and the methods each path serves. */
class RoutesTest {
    private fun request(method: String, path: String, headers: List<Pair<String, String>> = emptyList(), body: String = "") =
        Request(method, path, headers) { body.toByteArray() }

    private val origin = "http://127.0.0.1:3101"
    private val settings = LoginSettings.read(mapOf("BETTER_AUTH_SECRET" to "s", "AUTH_URL" to origin))

    @Test
    fun `outside the comparison every request is 404 notFound`() {
        val backend = Backend(false, { error("never opened") }, { settings })
        for ((method, path) in listOf("GET" to "/api/health", "GET" to "/api/workspace", "PUT" to "/api/workspace",
            "POST" to "/api/workspace/records", "GET" to "/")) {
            val response = backend.handle(request(method, path))
            assertEquals(404, response.status)
            assertEquals("{\"error\":\"notFound\"}\n", response.text())
            assertEquals("kotlin", response.header("x-typle-backend"))
        }
    }

    @Test
    fun `other methods are 405 with Allow, as the route files answer them`() {
        val backend = Backend(true, { null }, { settings })
        val cases = mapOf(
            "/api/health" to (listOf("POST", "PUT", "PATCH", "DELETE", "OPTIONS") to "GET, HEAD"),
            "/api/workspace" to (listOf("POST", "PATCH", "DELETE", "OPTIONS") to "GET, HEAD, PUT"),
            "/api/workspace/records" to (listOf("GET", "HEAD", "PUT", "PATCH", "OPTIONS") to "POST, DELETE"),
        )
        for ((path, methodsAllow) in cases) {
            for (method in methodsAllow.first) {
                val response = backend.handle(request(method, path))
                assertEquals(405, response.status, "$method $path")
                assertEquals(methodsAllow.second, response.header("Allow"), "$method $path")
                assertEquals("{\"error\":\"methodNotAllowed\"}\n", response.text())
                assertEquals("kotlin", response.header("x-typle-backend"))
                assertEquals("private, no-store", response.header("Cache-Control"))
            }
        }
    }

    @Test
    fun `without a database the health is 503, and a write is refused before it is opened`() {
        val backend = Backend(true, { null }, { settings })
        val health = backend.handle(request("GET", "/api/health"))
        assertEquals(503, health.status)
        assertEquals("{\"error\":\"databaseUnavailable\"}\n", health.text())
        val head = backend.handle(request("HEAD", "/api/health"))
        assertEquals(503, head.status)
        assertNull(head.body)
        val evil = backend.handle(request("PUT", "/api/workspace", listOf("Origin" to "https://evil.example", "Content-Type" to "application/json"), "{}"))
        assertEquals(403, evil.status)
        assertEquals("{\"error\":\"forbiddenRequest\"}\n", evil.text())
        val text = backend.handle(request("POST", "/api/workspace/records", listOf("Origin" to origin, "Content-Type" to "Application/JSON"), "{}"))
        assertEquals(403, text.status)
        // The iPad and Mac apps' bearer tokens are gone from typle-r: an Authorization header does not stand in for the Origin.
        for (path in listOf("/api/workspace" to "PUT", "/api/workspace/records" to "POST")) {
            val bearer = backend.handle(request(path.second, path.first, listOf("Authorization" to "Bearer x", "Content-Type" to "application/json"), "{}"))
            assertEquals(403, bearer.status, path.first)
            assertEquals("{\"error\":\"forbiddenRequest\"}\n", bearer.text())
        }
        val noAuth = Backend(true, { null }, { null }).handle(request("GET", "/api/workspace"))
        assertEquals("{\"error\":\"authUnavailable\"}\n", noAuth.text())
        assertEquals(503, backend.handle(request("GET", "/api/workspace")).status)
    }

    @Test
    fun `a postgres URL becomes a JDBC URL, TLS checked as node-postgres checks it`() {
        val neon = DatabaseUrl.toJdbc("postgresql://user:p%40ss@ep-x.ap-southeast-1.aws.neon.tech/neondb?sslmode=require&channel_binding=require")
        assertEquals("jdbc:postgresql://ep-x.ap-southeast-1.aws.neon.tech/neondb?sslmode=verify-full&sslfactory=org.postgresql.ssl.DefaultJavaSSLFactory", neon.url)
        assertEquals("user", neon.user)
        assertEquals("p@ss", neon.password)
        assertEquals("jdbc:postgresql://127.0.0.1:5433/typle?sslmode=disable", DatabaseUrl.toJdbc("postgres://typle:x@127.0.0.1:5433/typle").url)
    }
}

/** The login without a database: Better Auth's cookie, read against sessions that are only known here. */
class AccountTest {
    private val secret = "test secret"
    private val origin = "http://127.0.0.1:3101"
    private val settings = LoginSettings.read(mapOf("BETTER_AUTH_SECRET" to secret, "AUTH_URL" to origin))!!
    private val owner = "github:9100000001"

    private fun tokenOf(owner: String) = "session-of:$owner"

    /** The sessions known: one of each account, by tokenOf. */
    private val sessions = SessionSource { token, _ ->
        if (token.startsWith("session-of:")) Session(token.removePrefix("session-of:"), "N", "") else null
    }

    private fun cookie(owner: String, secure: Boolean = false) = "${Sessions.cookieName(secure)}=${Sessions.sign(tokenOf(owner), secret)}"

    private fun request(cookie: String?, vararg headers: Pair<String, String>) =
        Request("GET", "/api/workspace", listOfNotNull(cookie?.let { "Cookie" to it }) + headers) { ByteArray(0) }

    private fun signedIn(request: Request, settings: LoginSettings = this.settings, source: SessionSource? = sessions) =
        Account.signedIn(request, source, settings)

    private fun expectRefusal(result: Account.SignedIn, status: Int, key: String) {
        val refusal = result as? Account.Refusal ?: error("signed in as ${(result as Account.User).user}")
        assertEquals(status, refusal.response.status)
        assertEquals("{\"error\":\"$key\"}\n", refusal.response.text())
        // Typle-r reads the session through auth.api.getSession, whose cookies never reach the answer.
        assertNull(refusal.response.header("Set-Cookie"))
    }

    private fun expectUser(result: Account.SignedIn, sub: String) {
        val user = result as? Account.User ?: error("refused: ${(result as Account.Refusal).response.text()}")
        assertEquals(sub, user.user.sub)
    }

    @Test
    fun `a Better Auth session opens the account, and no answer drops the cookie`() {
        expectUser(signedIn(request(cookie(owner))), owner)
        expectRefusal(signedIn(request(null)), 401, "loginRequired")
        expectRefusal(signedIn(request("${Sessions.cookieName(false)}=${Sessions.sign("no-session", secret)}")), 401, "loginRequired")
        expectRefusal(signedIn(request("${Sessions.cookieName(false)}=${Sessions.sign(tokenOf(owner), "another secret")}")), 401, "loginRequired")
        // Over HTTPS the cookie is the __Secure- one, and the plain one is no one.
        val https = LoginSettings.read(mapOf("BETTER_AUTH_SECRET" to secret, "AUTH_URL" to "https://typle.example"))!!
        expectUser(signedIn(request(cookie(owner, true)), https), owner)
        expectRefusal(signedIn(request(cookie(owner, false)), https), 401, "loginRequired")
    }

    @Test
    fun `an Authorization header is ignored`() {
        expectRefusal(signedIn(request(null, "Authorization" to "Bearer x")), 401, "loginRequired")
        expectUser(signedIn(request(cookie(owner), "Authorization" to "Bearer x")), owner)
    }

    @Test
    fun `when the sessions cannot be read, the data is not served`() {
        expectRefusal(signedIn(request(cookie(owner)), source = SessionSource { _, _ -> throw IllegalStateException("down") }), 503, "databaseUnavailable")
        expectRefusal(signedIn(request(cookie(owner)), source = null), 503, "databaseUnavailable")
        // Without a valid cookie there is nothing to read: the login is required.
        expectRefusal(signedIn(request(null), source = null), 401, "loginRequired")
    }

    @Test
    fun `without BETTER_AUTH_SECRET every request that needs the login is 503 databaseUnavailable`() {
        val noSecret = LoginSettings.read(mapOf("BETTER_AUTH_SECRET" to "  ", "AUTH_URL" to origin))!!
        expectRefusal(signedIn(request(cookie(owner)), noSecret), 503, "databaseUnavailable")
        expectRefusal(signedIn(request(null), noSecret), 503, "databaseUnavailable")
    }

    @Test
    fun `the shared demo account's sessions work only while its login is on (DEMO_LOGIN=1)`() {
        expectRefusal(signedIn(request(cookie(Account.DEMO_ACCOUNT_ID))), 401, "loginRequired")
        val on = LoginSettings.read(mapOf("BETTER_AUTH_SECRET" to secret, "AUTH_URL" to origin, "DEMO_LOGIN" to "1"))!!
        expectUser(signedIn(request(cookie(Account.DEMO_ACCOUNT_ID)), on), Account.DEMO_ACCOUNT_ID)
    }

    @Test
    fun `the cookie is the first of its name, a leading quote dropped with the last character`() {
        val name = Sessions.cookieName(false)
        val good = Sessions.sign(tokenOf(owner), secret)
        for ((header, opens) in mapOf(
            "$name=$good" to true,
            "a=b; $name=\"$good\"" to true,
            " $name = $good ;x=y" to true,
            "$name=bad; $name=$good" to false,
            "__Secure-$name=$good" to false,
            "better-auth.session_data=$good" to false,
            "" to false,
        )) {
            val result = signedIn(request(header))
            assertEquals(opens, result is Account.User, "Cookie $header")
        }
    }
}

/**
 * The scenarios the Go backend ran against a real database (tests/fixtures/workspace-writes.json and record-writes.json),
 * run here against TYPLE_TEST_DATABASE_URL; skipped when it is not set.
 */
@TestInstance(TestInstance.Lifecycle.PER_CLASS)
class DatabaseTest {
    private val url = System.getenv("TYPLE_TEST_DATABASE_URL")?.ifEmpty { null }
    private val db: Database? by lazy { url?.let(Database::open) }
    private val owners = ArrayList<String>()
    private val secret = "typle-kotlin-test-secret"
    private val origin = "http://127.0.0.1:3101"
    private val settings = LoginSettings.read(mapOf("BETTER_AUTH_SECRET" to secret, "AUTH_URL" to origin))!!

    private fun database(): Database {
        assumeTrue(url != null, "TYPLE_TEST_DATABASE_URL is not set")
        return db!!
    }

    @AfterAll
    fun cleanUp() {
        val db = db ?: return
        if (url == null) return
        db.withConnection { c ->
            for (table in listOf("typle_records", "typle_workspaces")) {
                c.execute("DELETE FROM $table WHERE owner_id = ANY(?)", c.createArrayOf("text", owners.toTypedArray()))
            }
            // The session rows go with their users (ON DELETE CASCADE).
            c.execute("DELETE FROM typle_auth_users WHERE id = ANY(?)", c.createArrayOf("text", owners.toTypedArray()))
        }
        db.source.close()
    }

    /**
     * A new random account with its Better Auth user and a session of a day, whose token is the one cookieOf names.
     * Its rows are deleted when the tests end.
     */
    private fun newOwner(): String {
        val owner = "github:${Random.nextLong(1_000_000_000, 2_000_000_000)}".also { synchronized(owners) { owners.add(it) } }
        database().withConnection { c ->
            c.execute("INSERT INTO typle_auth_users (id, name, email, email_verified) VALUES (?, 'Test', ?, true)", owner, "$owner@users.invalid")
            c.execute("INSERT INTO typle_auth_sessions (id, token, user_id, expires_at) VALUES (?, ?, ?, now() + interval '1 day')",
                owner, tokenOf(owner), owner)
        }
        return owner
    }

    private fun tokenOf(owner: String) = "session-of:$owner"

    private fun cookieOf(owner: String) = "${Sessions.cookieName(false)}=${Sessions.sign(tokenOf(owner), secret)}"

    private fun request(method: String, path: String, owner: String?, body: String, headers: Map<String, String> = emptyMap()): Request {
        val all = LinkedHashMap(mapOf("Origin" to origin, "Content-Type" to "application/json"))
        if (owner != null) all["Cookie"] = cookieOf(owner)
        all.putAll(headers)
        return Request(method, path, all.filterValues { it.isNotEmpty() }.toList()) { body.toByteArray() }
    }

    private fun insertRow(owner: String, row: JsObject) {
        database().withConnection { c ->
            c.execute(
                """INSERT INTO typle_workspaces (owner_id, lists, packs, devices, interests, revision, records_split)
        VALUES (?, ?::jsonb, ?::jsonb, ?::jsonb, ?::jsonb, ?, ?)""",
                owner, JsonWriter.stringify(row["lists"]), row["packs"]?.let(JsonWriter::stringify), JsonWriter.stringify(row["devices"]),
                row["interests"]?.let(JsonWriter::stringify), (row["revision"] as Double).toInt(), row["split"] as Boolean,
            )
        }
    }

    private fun state(owner: String): Any? = database().withConnection { c ->
        val row = c.query(
            """SELECT COALESCE((SELECT jsonb_build_object('lists', lists, 'packs', packs, 'devices', devices, 'interests', interests,
    'revision', revision, 'split', records_split) FROM typle_workspaces WHERE owner_id = ?), 'null'::jsonb)::text AS v""",
            owner,
        ) { it.next(); it.getString(1) }
        val records = c.query(
            """SELECT COALESCE(jsonb_agg(jsonb_build_array(list_id, record_id, position, (extract(epoch FROM at) * 1000)::bigint, data)
    ORDER BY list_id, position), '[]'::jsonb)::text AS v FROM typle_records WHERE owner_id = ?""",
            owner,
        ) { it.next(); it.getString(1) }
        mapOf("row" to JsonParser.parse(row), "records" to JsonParser.parse(records))
    }

    /** A save of body as /api/workspace answers it: ok, or the key it is refused with. */
    private fun save(owner: String, body: JsObject): String {
        val read = Workspace.readWorkspaceChange(JsObject(body).apply { put("accountId", owner) })
        if (read is Workspace.ChangeRefused) return read.key
        read as Workspace.ChangeOk
        return try {
            Workspace.writeWorkspace(database(), owner, read.revision, read.change)
            "ok"
        } catch (error: Conflict) {
            "conflict"
        } catch (error: AppError) {
            error.key
        }
    }

    @TestFactory
    fun `workspace writes as the Go backend (workspace-writes-json)`() = objects(Fixtures.obj("workspace-writes.json")["scenarios"]).map { scenario ->
        DynamicTest.dynamicTest(scenario["name"] as String) {
            database()
            val owner = newOwner()
            ((scenario["setup"] as JsObject?)?.get("row") as JsObject?)?.let { insertRow(owner, it) }
            for ((index, step) in objects(scenario["steps"]).withIndex()) {
                assertEquals(step["result"], save(owner, step["body"] as JsObject), "step ${index + 1}")
                assertJsonEquals(mapOf("row" to step["row"], "records" to step["records"]), state(owner), "step ${index + 1}")
            }
        }
    }

    @TestFactory
    fun `record writes as the Go backend (record-writes-json)`() = objects(Fixtures.obj("record-writes.json")["scenarios"]).map { scenario ->
        DynamicTest.dynamicTest(scenario["name"] as String) {
            val db = database()
            val owner = newOwner()
            ((scenario["setup"] as JsObject)["row"] as JsObject?)?.let { insertRow(owner, it) }
            for ((index, step) in objects(scenario["steps"]).withIndex()) {
                val body = JsObject().apply { put("accountId", owner); putAll(step["body"] as JsObject) }
                val response = Workspace.answerRecords(
                    request(if (step["kind"] == "add") "POST" else "DELETE", "/api/workspace/records", owner, JsonWriter.stringify(body)),
                    { db }, { settings },
                )
                val answer = JsonParser.parse(response.text()) as JsObject
                assertEquals((step["status"] as Double).toInt(), response.status, "step ${index + 1}: ${response.text()}")
                if (step["key"] != "") assertJsonEquals(mapOf("error" to step["key"]), answer, "step ${index + 1}")
                if (step["kind"] == "add" && step["key"] == "") assertJsonEquals(step["record"], answer["record"], "step ${index + 1}")
                assertJsonEquals(mapOf("row" to step["row"], "records" to step["records"]), state(owner), "step ${index + 1}")
            }
        }
    }

    private val at = "2026-09-07T00:00:00.000Z"
    private fun library(name: String) =
        """"packs":[{"id":"p","name":"P","createdAt":"$at"}],"lists":[{"id":"l","name":"$name","packId":"p","words":[{"display":"a","input":"a","annotation":""}],"records":[],"createdAt":"$at"}]"""

    private fun expectError(response: Response, status: Int, key: String) {
        assertEquals(status, response.status, response.text())
        assertEquals("{\"error\":\"$key\"}\n", response.text())
    }

    @Test
    fun `an account that has saved nothing reads as revision 0, and a save is read back`() {
        val db = database()
        val owner = newOwner()
        val open = { db }
        val login = { settings }
        val empty = Workspace.answerWorkspace(request("GET", "/api/workspace", owner, ""), open, login)
        assertEquals(200, empty.status)
        assertJsonEquals(mapOf("lists" to JsArray(), "packs" to null, "devices" to JsArray(), "interests" to Interests.DEFAULT_INTERESTS,
            "revision" to 0, "accountId" to owner), JsonParser.parse(empty.text()))
        val saved = Workspace.answerWorkspace(request("PUT", "/api/workspace", owner, """{"revision":0,"accountId":"$owner",${library("one")}}"""), open, login)
        assertEquals(200, saved.status, saved.text())
        val answer = JsonParser.parse(saved.text()) as JsObject
        assertEquals(1.0, answer["revision"])
        assertEquals(saved.text(), Workspace.answerWorkspace(request("GET", "/api/workspace", owner, ""), open, login).text())
        expectError(Workspace.answerWorkspace(request("PUT", "/api/workspace", owner, """{"revision":0,"accountId":"$owner",${library("two")}}"""), open, login), 409, "conflict")
        // PUT always writes: the same workspace sent back at its revision moves the revision on.
        val again = Workspace.answerWorkspace(request("PUT", "/api/workspace", owner, """{"revision":1,"accountId":"$owner",${library("one")}}"""), open, login)
        assertEquals(2.0, (JsonParser.parse(again.text()) as JsObject)["revision"])
    }

    @Test
    fun `a save is refused as the Go backend refused it`() {
        val db = database()
        val owner = newOwner()
        val open = { db }
        val login = { settings }
        val good = """{"revision":0,"accountId":"$owner",${library("one")}}"""
        fun put(body: String, headers: Map<String, String> = emptyMap(), who: String? = owner) =
            Workspace.answerWorkspace(request("PUT", "/api/workspace", who, body, headers), open, login)
        expectError(put(good, mapOf("Origin" to "https://evil.example")), 403, "forbiddenRequest")
        expectError(put(good, mapOf("Content-Type" to "text/plain")), 403, "forbiddenRequest")
        expectError(put(good, who = null), 401, "loginRequired")
        expectError(put(""), 400, "emptyRequest")
        expectError(put("""{"x":"${"a".repeat(1 shl 20)}"}"""), 413, "workspaceTooLarge")
        expectError(put("[]"), 400, "invalidWorkspace")
        expectError(put("""{"revision":0,"accountId":"github:2",${library("one")}}"""), 401, "loginAccountChanged")
        // An Authorization header no longer stands in for the Origin.
        expectError(put(good, mapOf("Origin" to "", "Authorization" to "Bearer x")), 403, "forbiddenRequest")
    }

    @Test
    fun `a session row is read by its token while it has not expired`() {
        val db = database()
        val owner = newOwner()
        val sessions = Account.databaseSessions(db)
        val now = Instant.now()
        assertEquals(Session(owner, "Test", ""), sessions.sessionUser(tokenOf(owner), now))
        assertNull(sessions.sessionUser(tokenOf(owner) + "x", now))
        assertNull(sessions.sessionUser(tokenOf(owner), now.plus(Duration.ofDays(2))))
    }

    @Test
    fun `a session deleted by signing out of every device is refused at once, without dropping the cookie`() {
        val db = database()
        val owner = newOwner()
        assertEquals(200, Workspace.answerWorkspace(request("GET", "/api/workspace", owner, ""), { db }, { settings }).status)
        db.withConnection { it.execute("DELETE FROM typle_auth_sessions WHERE user_id = ?", owner) }
        val response = Workspace.answerWorkspace(request("GET", "/api/workspace", owner, ""), { db }, { settings })
        expectError(response, 401, "loginRequired")
        assertNull(response.header("Set-Cookie"))
    }

    @Test
    fun `an expired session is refused`() {
        val db = database()
        val owner = newOwner()
        db.withConnection { it.execute("UPDATE typle_auth_sessions SET expires_at = now() - interval '1 second' WHERE user_id = ?", owner) }
        expectError(Workspace.answerWorkspace(request("GET", "/api/workspace", owner, ""), { db }, { settings }), 401, "loginRequired")
    }

    @Test
    fun `of two saves from the same revision, only one is saved`() {
        database()
        val owner = newOwner()
        fun body(revision: Int, name: String) = JsonParser.parse("""{"revision":$revision,${library(name)}}""") as JsObject
        val pool = Executors.newFixedThreadPool(2)
        try {
            val first = pool.invokeAll(listOf(Callable { save(owner, body(0, "one")) }, Callable { save(owner, body(0, "two")) })).map { it.get() }
            assertEquals(listOf("conflict", "ok"), first.sorted())
            val second = pool.invokeAll(listOf(Callable { save(owner, body(1, "three")) }, Callable { save(owner, body(1, "four")) })).map { it.get() }
            assertEquals(listOf("conflict", "ok"), second.sorted())
        } finally {
            pool.shutdown()
        }
        assertTrue(((state(owner) as Map<*, *>)["row"] as JsObject)["revision"] == 2.0)
    }

    @Test
    fun `the health answers 200 once the database answers`() {
        val db = database()
        val response = Backend(true, { db }, { settings }).handle(Request("GET", "/api/health", emptyList()) { ByteArray(0) })
        assertEquals(200, response.status)
        assertEquals("{\"status\":\"ok\"}\n", response.text())
    }
}
