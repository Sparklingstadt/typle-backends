package typle

import typle.lib.JsArray
import typle.lib.JsObject
import typle.lib.JsonParser
import typle.lib.JsonWriter
import java.io.File
import kotlin.test.assertEquals

// The shared fixtures of the repository (tests/fixtures), read in place: the Go backend and the TypeScript code are
// held to the same files.

object Fixtures {
    val repository = File(System.getProperty("typle.repository") ?: "..")
    val dir = File(repository, "tests/fixtures")

    fun read(path: String): Any? = JsonParser.parse(File(dir, path).readText())
    fun obj(path: String) = read(path) as JsObject

    /** The JSON files of a fixture directory, by name. */
    fun files(path: String): List<Pair<String, JsObject>> =
        File(dir, path).listFiles()!!.filter { it.name.endsWith(".json") }.sortedBy { it.name }
            .map { it.name to JsonParser.parse(it.readText()) as JsObject }
}

/** value as JSON reads it back: numbers as doubles, key order irrelevant (assert.deepEqual compares so). */
fun normalized(value: Any?): Any? = JsonParser.parse(JsonWriter.stringify(value))

fun assertJsonEquals(expected: Any?, actual: Any?, message: String? = null) =
    assertEquals(normalized(expected), normalized(actual), message)

@Suppress("UNCHECKED_CAST")
fun objects(value: Any?): List<JsObject> = (value as JsArray).map { it as JsObject }
