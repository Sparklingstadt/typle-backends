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
import typle.server.Request
import typle.server.Response
import typle.server.Workspace
import typle.server.execute
import typle.server.query
import java.util.Base64
import java.util.concurrent.Callable
import java.util.concurrent.Executors
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec
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
    private val settings = LoginSettings.read(mapOf("AUTH_SECRET" to "s", "AUTH_URL" to origin))

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
    private val settings = LoginSettings.read(mapOf("AUTH_SECRET" to secret, "AUTH_URL" to origin))!!

    private fun database(): Database {
        assumeTrue(url != null, "TYPLE_TEST_DATABASE_URL is not set")
        return db!!
    }

    @AfterAll
    fun cleanUp() {
        val db = db ?: return
        if (url == null) return
        db.withConnection { c ->
            for (table in listOf("typle_session_revocations", "typle_records", "typle_workspaces")) {
                c.execute("DELETE FROM $table WHERE owner_id = ANY(?)", c.createArrayOf("text", owners.toTypedArray()))
            }
        }
        db.source.close()
    }

    private fun newOwner() = "github:${Random.nextLong(1_000_000_000, 2_000_000_000)}".also { synchronized(owners) { owners.add(it) } }

    private fun seal(info: String, sub: String): String {
        val key = LoginSettings.deriveKey(secret, info)
        val now = System.currentTimeMillis() / 1000
        val payload = Base64.getUrlEncoder().withoutPadding()
            .encodeToString("""{"sub":"$sub","name":"N","image":"","iat":${now - 60},"exp":${now + 3600}}""".toByteArray())
        val mac = Mac.getInstance("HmacSHA256").apply { init(SecretKeySpec(key, "HmacSHA256")) }.doFinal(payload.toByteArray())
        return "$payload.${Base64.getUrlEncoder().withoutPadding().encodeToString(mac)}"
    }

    private fun cookieOf(owner: String) = "typle-session=${seal("typle session v1", owner)}"

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
        // An app's token stands without an Origin.
        val app = Workspace.answerWorkspace(
            Request("PUT", "/api/workspace", listOf("Authorization" to "Bearer ${seal("typle app session v1", owner)}", "Content-Type" to "application/json")) { good.toByteArray() },
            open, login,
        )
        assertEquals(200, app.status, app.text())
    }

    @Test
    fun `a revoked login is refused, and a browser loses its cookie`() {
        val db = database()
        val owner = newOwner()
        db.withConnection { it.execute("INSERT INTO typle_session_revocations (owner_id, revoked_at) VALUES (?, now())", owner) }
        val response = Workspace.answerWorkspace(request("GET", "/api/workspace", owner, ""), { db }, { settings })
        expectError(response, 401, "loginRequired")
        assertEquals("typle-session=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax", response.header("Set-Cookie"))
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
