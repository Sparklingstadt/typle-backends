package typle

import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.TestFactory
import typle.lib.AppError
import typle.lib.Catalog
import typle.lib.FollowedPacks
import typle.lib.Interests
import typle.lib.JsArray
import typle.lib.JsDate
import typle.lib.JsObject
import typle.lib.JsonParser
import typle.lib.JsonWriter
import typle.lib.Library
import typle.lib.Limits
import typle.lib.OfficialPack
import typle.lib.Records
import typle.lib.WordPacks
import typle.lib.WorkspaceRequest
import typle.lib.goFloat
import typle.lib.jsNumber
import typle.lib.prop
import typle.server.LoginSettings
import typle.server.RecordStore
import typle.server.Sessions
import typle.server.Workspace
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

class LibTest {
    @Test
    fun `the limits are the ones of tests-fixtures-limits-json`() {
        val limits = Fixtures.obj("limits.json")
        for ((name, value) in limits) {
            if (name == "MAX_IMPORT_BYTES") continue
            assertEquals((value as Double).toInt(), Limits.byName[name], name)
        }
    }

    @Test
    fun `the default interests are those of tests-fixtures-default-interests-json`() {
        assertJsonEquals(Fixtures.read("default-interests.json"), Interests.DEFAULT_INTERESTS)
    }

    @Test
    fun `save bodies are accepted and refused as the backend does (workspace-put)`() {
        for (case in Fixtures.obj("workspace-put.json")["cases"] as JsArray) {
            case as JsObject
            val name = case["name"] as String
            val accepted = case["accepted"] as Boolean
            val body = JsonParser.parse(case["body"] as String) as? JsObject
            if (body == null) {
                assertTrue(!accepted, name)
                continue
            }
            val change = try { WorkspaceRequest.validate(body) } catch (_: Exception) { null }
            assertEquals(accepted, change != null, name)
            if (change != null) {
                assertEquals(case["revision"], change.first, name)
                assertEquals((case["lists"] as Double).toInt(), change.second.lists?.size ?: -1, name)
                assertEquals((case["devices"] as Double).toInt(), change.second.devices?.size ?: -1, name)
            }
            val read = Workspace.readWorkspaceChange(body)
            assertEquals(accepted, read is Workspace.ChangeOk, name)
            if (read is Workspace.ChangeRefused) assertTrue(read.key in listOf("invalidWorkspace", "levelsConflict"), name)
        }
    }

    private fun body(fields: String) = JsonParser.parse("""{"revision":1,"accountId":"github:1",$fields}""") as JsObject
    private val list = """{"id":"a","name":"a","createdAt":"2026-09-06","words":[{"display":"a","input":"a","annotation":""}],"records":[]}"""

    @Test
    fun `a body is refused with invalidWorkspace for what the Go backend refused`() {
        for (fields in listOf(
            """"profileId":"p","lists":[$list]""", """"packs":[]""", """"lists":null""", """"lists":[$list],"packs":null""",
            """"devices":null""", """"lists":[${list.replace("\"name\":\"a\"", "\"name\":\"a\\u0000b\"")}]""",
            """"lists":[${list.replace("\"name\":\"a\"", "\"name\":\"a\\ud800b\"")}]""",
            """"devices":[{"id":"d","name":"x\udc00","layout":""}]""", """"interests":["a\u0000"]""",
            """"lists":[${list.replace("\"words\":[{\"display\":\"a\",\"input\":\"a\",\"annotation\":\"\"}]", "\"words\":[]")}]""",
            """"lists":"x"""",
        )) {
            val read = Workspace.readWorkspaceChange(body(fields))
            assertTrue(read is Workspace.ChangeRefused && read.key == "invalidWorkspace", fields)
        }
        assertTrue(Workspace.readWorkspaceChange(body(""""lists":[${list.replace("\"name\":\"a\"", "\"name\":\"a\\ud83d\\ude00\"")}]""")) is Workspace.ChangeOk)
    }

    @Test
    fun `interests sent as null are none, and lists of several levels sent with packs are taken`() {
        val none = Workspace.readWorkspaceChange(body(""""interests":null""")) as Workspace.ChangeOk
        assertEquals(JsArray(), none.change.interests)
        val packs = """"packs":[{"id":"p","name":"P","createdAt":"2026-09-06"}]"""
        val several = Workspace.readWorkspaceChange(body(
            """$packs,"lists":[${list.replace("\"records\":[]", "\"records\":[],\"packId\":\"p\",\"levels\":[\"初級\",\"文法\"]")}]""",
        ))
        assertTrue(several is Workspace.ChangeOk)
        assertJsonEquals(listOf("初級", "文法"), several.change.lists!![0]["levels"])
        val overlap = Workspace.readWorkspaceChange(body(
            """$packs,"lists":[${list.replace("\"id\":\"a\",\"name\":\"a\"", "\"id\":\"x\",\"name\":\"x\"").replace("\"records\":[]", "\"records\":[],\"packId\":\"p\"")},""" +
                """${list.replace("\"id\":\"a\",\"name\":\"a\"", "\"id\":\"y\",\"name\":\"y\"").replace("\"records\":[]", "\"records\":[],\"packId\":\"p\",\"levels\":[\"x\"]")}]""",
        ))
        assertTrue(overlap is Workspace.ChangeRefused && overlap.key == "levelsConflict")
    }

    @TestFactory
    fun `pack fixtures (tests-fixtures-packs)`() = Fixtures.files("packs").map { (name, fixture) ->
        DynamicTest.dynamicTest(name) {
            val packs = fixture["packs"]?.let { objects(it) }
            val result = WordPacks.normalizeLibrary(objects(fixture["lists"]), packs)
            assertJsonEquals(fixture["expected"], mapOf("packs" to result.packs, "lists" to result.lists))
            WordPacks.validateLibrary(result)
            val again = WordPacks.normalizeLibrary(result.lists, result.packs)
            assertJsonEquals(mapOf("packs" to result.packs, "lists" to result.lists), mapOf("packs" to again.packs, "lists" to again.lists))
        }
    }

    @TestFactory
    fun `follow fixtures (tests-fixtures-follow)`() = Fixtures.files("follow").map { (name, fixture) ->
        DynamicTest.dynamicTest(name) {
            val official = objects(fixture["official"]).map {
                OfficialPack(it["catalogId"] as String, it["version"] as Double, it["pack"] as JsObject, objects(it["lists"]))
            }
            val library = fixture["library"] as JsObject
            val packs = library["packs"]?.let { objects(it) }
            val normalized = WordPacks.normalizeLibrary(objects(library["lists"]), packs)
            val once = FollowedPacks.followOfficial(normalized, official)
            assertJsonEquals(fixture["expected"], mapOf("packs" to once.packs, "lists" to once.lists))
            val twice = FollowedPacks.followOfficial(once, official)
            assertJsonEquals(mapOf("packs" to once.packs, "lists" to once.lists), mapOf("packs" to twice.packs, "lists" to twice.lists))
            WordPacks.validateLibrary(once)
        }
    }

    private fun errorKey(name: Any?) = if (name == "listNotFound") "recordListDeleted" else name as String

    @TestFactory
    fun `add-record fixtures (tests-fixtures-records)`() = Fixtures.files("records").map { (name, fixture) ->
        DynamicTest.dynamicTest(name) {
            val lists = objects(fixture["lists"])
            val run = { Records.addRecord(lists, fixture["devices"] as JsArray, fixture["listId"] as String, fixture["input"] as JsObject) }
            val expected = fixture["expected"] as JsObject
            if (expected["error"] != null) {
                assertEquals(errorKey(expected["error"]), assertFailsWith<AppError> { run() }.key)
                return@dynamicTest
            }
            val result = run()
            assertEquals(expected["added"], result.added)
            assertJsonEquals(expected["record"], result.record)
            val saved = result.lists.first { it["id"] == fixture["listId"] }["records"] as JsArray
            assertJsonEquals(expected["record"], saved.last())
            val before = (lists.first { it["id"] == fixture["listId"] }["records"] as JsArray).size
            assertEquals(before + if (result.added) 1 else 0, saved.size)
        }
    }

    @TestFactory
    fun `record operation fixtures (tests-fixtures-record-operations)`() = Fixtures.files("record-operations").map { (name, fixture) ->
        DynamicTest.dynamicTest(name) {
            var lists = objects(fixture["lists"])
            val devices = fixture["devices"] as JsArray
            for ((i, item) in (fixture["steps"] as JsArray).withIndex()) {
                val step = item as JsObject
                val expected = step["expected"] as JsObject
                if (step["op"] == "add") {
                    if (expected["error"] != null) {
                        val error = assertFailsWith<AppError>("step $i") { Records.addRecord(lists, devices, step["listId"] as String, step["input"] as JsObject) }
                        assertEquals(errorKey(expected["error"]), error.key, "step $i")
                        continue
                    }
                    val result = Records.addRecord(lists, devices, step["listId"] as String, step["input"] as JsObject)
                    assertEquals(expected["added"], result.added, "step $i")
                    lists = result.lists
                } else {
                    val result = Records.deleteRecord(lists, step["listId"] as String, step["recordId"] as String)
                    assertEquals(expected["deleted"], result != null, "step $i")
                    if (result != null) lists = result
                }
            }
            for ((listId, records) in fixture["expected"] as JsObject) {
                assertJsonEquals(records, lists.first { it["id"] == listId }["records"], listId)
            }
        }
    }

    // Values better-call's signCookieValue (better-call 1.x in typle-r v2.68.1) signed with this secret.
    private val fixtureSecret = "typle-fixture-secret"
    private val signed = listOf(
        "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj" to "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D",
        "a.b+c/d=e" to "a.b%2Bc%2Fd%3De.QgtT95hjCvQfprQC%2BUtI5LsqCC549MK9KWYHFNPmUuE%3D",
    )

    @Test
    fun `a session cookie is signed as better-call signs it`() {
        for ((token, value) in signed) assertEquals(value, Sessions.sign(token, fixtureSecret), token)
    }

    @Test
    fun `the token of a session cookie is read as better-call reads it`() {
        for ((token, value) in signed) assertEquals(token, Sessions.token(value, fixtureSecret), value)
        // The value as it is, not URL-encoded, opens too: decoding leaves it as it is.
        assertEquals(signed[0].first, Sessions.token("Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK+RyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w=", fixtureSecret))
        // A value that is not valid percent-encoding is read as it is (tryDecode).
        assertNull(Sessions.token("%zz." + signed[0].second, fixtureSecret))
    }

    @Test
    fun `a session cookie that is not signed with the secret is no one`() {
        val good = signed[0].second
        for ((name, value) in mapOf(
            "empty" to "",
            "no dot" to "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj",
            "dot first" to ".HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D",
            "other token" to "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHk.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D",
            "short" to "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w",
            "no padding end" to "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2wA",
            "not base64" to "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK-RyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D",
        )) {
            assertNull(Sessions.token(value, fixtureSecret), name)
        }
        assertNull(Sessions.token(good, "another secret"))
        assertNull(Sessions.token(good, ""))
    }

    @Test
    fun `the session cookie is better-auth's, __Secure- over HTTPS`() {
        assertEquals("better-auth.session_token", Sessions.cookieName(false))
        assertEquals("__Secure-better-auth.session_token", Sessions.cookieName(true))
        val https = LoginSettings.read(mapOf("BETTER_AUTH_SECRET" to " s \n", "AUTH_URL" to "https://typle.example/path"))!!
        assertEquals("__Secure-better-auth.session_token", https.cookieName)
        assertEquals("https://typle.example", https.origin)
        assertEquals("s", https.secret)
        assertEquals(false, https.demoLogin)
        val http = LoginSettings.read(mapOf("AUTH_URL" to "http://127.0.0.1:3101", "DEMO_LOGIN" to "1"))!!
        assertEquals("better-auth.session_token", http.cookieName)
        assertEquals("", http.secret)
        assertEquals(true, http.demoLogin)
        assertEquals(false, LoginSettings.read(mapOf("AUTH_URL" to "http://x", "DEMO_LOGIN" to "true"))!!.demoLogin)
        assertNull(LoginSettings.read(mapOf("BETTER_AUTH_SECRET" to "s", "AUTH_URL" to "ftp://typle.example")))
        assertNull(LoginSettings.read(mapOf("BETTER_AUTH_SECRET" to "s")))
    }

    @Test
    fun `numbers are written as Go writes them with %g, and ids as the Go backend gave them (record-ids-json)`() {
        val fixture = Fixtures.obj("record-ids.json")
        for (item in fixture["floats"] as JsArray) assertEquals(prop(item, "go"), goFloat(prop(item, "value") as Double), "${prop(item, "value")}")
        for (item in fixture["ids"] as JsArray) {
            item as JsObject
            assertEquals(item["id"], RecordStore.legacyId(item["listId"] as String, (item["position"] as Double).toInt(), item["date"] as String, item["time"] as Double))
        }
    }

    @Test
    fun `JSON is read and written as JSON-parse and JSON-stringify do`() {
        for ((number, text) in listOf(1.0 to "1", -0.0 to "0", 1.5 to "1.5", 1e21 to "1e+21", 1e20 to "100000000000000000000", 1e-7 to "1e-7",
            0.000001 to "0.000001", 123456789012345680000.0 to "123456789012345680000", 0.1 + 0.2 to "0.30000000000000004", 5e-324 to "5e-324",
            1.7976931348623157e308 to "1.7976931348623157e+308", 2.5e-5 to "0.000025", 1.25e22 to "1.25e+22")) {
            assertEquals(text, jsNumber(number), "$number")
        }
        assertEquals("""{"0":1,"2":2,"b":3,"a":4}""", JsonWriter.stringify(JsonParser.parse("""{"b":3,"2":2,"a":4,"0":1}""")))
        assertEquals("\"\\u0000\\ud800\\n😀é\\\"\\\\/\"", JsonWriter.stringify(JsonParser.parse("\"\\u0000\\ud800\\n😀é\\\"\\\\\\/\"")))
        for (bad in listOf("", "{", "[1,]", "{\"a\":1,}", "01", "1.", ".5", "+1", "'a'", "\"\t\"", "nul", "[1] x", "\"\\x\"")) {
            assertFailsWith<Exception>(bad) { JsonParser.parse(bad) }
        }
        assertEquals(Double.POSITIVE_INFINITY, JsonParser.parse("1e400"))
        assertEquals("""{"a":2}""", JsonWriter.stringify(JsonParser.parse("""{"a":1,"a":2}""")))
    }

    @Test
    fun `dates are read as Date-parse reads them`() {
        assertEquals(Date("2026-10-02T10:00:00.000Z"), JsDate.parse("2026-10-02T10:00:00.000Z"))
        assertEquals("2026-10-02T10:00:00.000Z", JsDate.toIsoString(JsDate.parse("2026-10-02T19:00:00+09:00")))
        assertEquals("2026-10-01T00:00:00.000Z", JsDate.toIsoString(JsDate.parse("2026-10-01")))
        assertEquals("2026-03-03T00:00:00.000Z", JsDate.toIsoString(JsDate.parse("2026-02-31")))
        assertEquals("2026-10-02T10:00:00.123Z", JsDate.toIsoString(JsDate.parse("2026-10-02T10:00:00.1239Z")))
        assertEquals("2026-09-07T10:00:00.000Z", JsDate.toIsoString(JsDate.parse("Mon, 07 Sep 2026 10:00:00 GMT")))
        assertEquals("2026-09-07T01:00:00.000Z", JsDate.toIsoString(JsDate.parse("Mon Sep 07 2026 10:00:00 GMT+0900 (JST)")))
        for (bad in listOf("not a date", "2026-13-01", "2026-10-01T25:00", "")) assertTrue(JsDate.parse(bad).isNaN(), bad)
    }

    private fun Date(iso: String) = java.time.Instant.parse(iso).toEpochMilli().toDouble()

    @Test
    fun `the official packs are built from src-lib-packs, and following them again changes nothing`() {
        val official = Catalog.official
        assertTrue(official.size > 10, "${official.size} official packs")
        for (entry in official) {
            assertTrue(entry.lists.isNotEmpty(), entry.catalogId)
            WordPacks.validateLibrary(Library(listOf(entry.pack), entry.lists))
        }
        val first = official[0]
        val pack = first.pack.without("origin").apply { put("id", "mine"); put("origin", JsObject(mapOf("catalogId" to first.catalogId, "version" to 1.0, "followed" to true))) }
        val lists = first.lists.map { JsObject(it).apply { put("packId", "mine"); put("origin", JsObject(mapOf("listId" to it["id"]))); put("id", "mine:${it["id"]}") } }
        val library = Library(listOf(pack), lists)
        val once = Catalog.followed(library)
        assertJsonEquals(mapOf("p" to once.packs, "l" to once.lists), mapOf("p" to Catalog.followed(once).packs, "l" to Catalog.followed(once).lists))
        assertEquals(first.lists.size, once.lists.size)
    }

    private fun JsObject.without(key: String) = JsObject(this).apply { remove(key) }
}
