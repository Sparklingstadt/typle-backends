package typle.lib

// The limits, errors, devices and interests of src/lib (limits.ts values are checked against tests/fixtures/limits.json).

/** An error named by a key of the app's `errors` messages (src/lib/appError.ts). */
class AppError(val key: String) : Exception(key)

/** A plain Error of the TypeScript code (new Error(...)): not an AppError, so it is answered by its caller's default. */
class PlainError(message: String) : Exception(message)

object Limits {
    const val MAX_LISTS = 150
    const val MAX_FOLLOWED_PACKS = 200
    const val MAX_LIST_NAME = 200
    const val MAX_WORDS = 1000
    const val MAX_RECORDS = 10000
    const val MAX_CATEGORY = 50
    const val MAX_TAG = 50
    const val MAX_TAGS = 20
    const val MAX_GROUP = 200
    const val MAX_LEVELS = 1
    const val MAX_SAVED_LEVELS = 5
    const val MAX_LEVEL = 50
    const val MAX_INTERESTS = 50
    const val MAX_INTEREST_LENGTH = 50
    const val MAX_PACK_NAME = 200
    const val MAX_PACK_DESCRIPTION = 500
    const val MAX_DEVICES = 100
    const val MAX_DEVICE_NAME = 100
    const val MAX_DEVICE_LAYOUT = 200
    const val MAX_MISSED_WORDS = 20
    const val MAX_WEAK_REVIEW_MISSED = 200
    const val MAX_WORD_TEXT = 1000
    const val MAX_ANNOTATION = 2000
    const val MAX_RECORD_ID = 100
    const val MAX_NAME = 200

    /** The limits by the names of tests/fixtures/limits.json (MAX_IMPORT_BYTES is the import's, not checked here). */
    val byName: Map<String, Int> = mapOf(
        "MAX_LISTS" to MAX_LISTS, "MAX_FOLLOWED_PACKS" to MAX_FOLLOWED_PACKS, "MAX_LIST_NAME" to MAX_LIST_NAME,
        "MAX_WORDS" to MAX_WORDS, "MAX_RECORDS" to MAX_RECORDS, "MAX_CATEGORY" to MAX_CATEGORY, "MAX_TAG" to MAX_TAG,
        "MAX_TAGS" to MAX_TAGS, "MAX_GROUP" to MAX_GROUP, "MAX_LEVELS" to MAX_LEVELS, "MAX_SAVED_LEVELS" to MAX_SAVED_LEVELS,
        "MAX_LEVEL" to MAX_LEVEL, "MAX_INTERESTS" to MAX_INTERESTS, "MAX_INTEREST_LENGTH" to MAX_INTEREST_LENGTH,
        "MAX_PACK_NAME" to MAX_PACK_NAME, "MAX_PACK_DESCRIPTION" to MAX_PACK_DESCRIPTION, "MAX_DEVICES" to MAX_DEVICES,
        "MAX_DEVICE_NAME" to MAX_DEVICE_NAME, "MAX_DEVICE_LAYOUT" to MAX_DEVICE_LAYOUT, "MAX_MISSED_WORDS" to MAX_MISSED_WORDS,
        "MAX_WORD_TEXT" to MAX_WORD_TEXT, "MAX_ANNOTATION" to MAX_ANNOTATION, "MAX_RECORD_ID" to MAX_RECORD_ID,
    )
}

/** src/lib/devices.ts. */
object Devices {
    const val DEFAULT_DEVICE_ID = "default"

    fun defaultDevice(name: String) = jsObject("id" to DEFAULT_DEVICE_ID, "name" to name, "layout" to "")

    fun isDevice(value: Any?): Boolean {
        if (!isObjectLike(value)) return false
        val id = prop(value, "id")
        val name = prop(value, "name")
        val layout = prop(value, "layout")
        return id is String && id.isNotEmpty() && id.length <= 100 &&
            name is String && name.jsTrim().isNotEmpty() && name.length <= Limits.MAX_DEVICE_NAME &&
            layout is String && layout.length <= Limits.MAX_DEVICE_LAYOUT
    }

    fun decodeDevices(value: Any?): JsArray {
        if (value !is JsArray || value.size > Limits.MAX_DEVICES || !value.all(::isDevice) ||
            value.any { prop(it, "id") == DEFAULT_DEVICE_ID } || value.map { prop(it, "id") }.toSet().size != value.size
        ) throw AppError("deviceDataInvalid")
        return jsArray(value.map {
            jsObject("id" to prop(it, "id"), "name" to (prop(it, "name") as String).jsTrim(), "layout" to (prop(it, "layout") as String).jsTrim())
        })
    }
}

/** src/lib/interests.ts: the ids of the options, in the order they are shown and saved. */
object Interests {
    private val options = listOf(
        "language" to listOf("zh", "ru", "ko", "en", "es", "fr", "de", "cs"),
        "programming" to listOf("go", "kotlin", "java", "typescript", "php", "ruby", "sql", "linux", "vim", "test", "node", "shell", "cloud"),
    )

    /** The interests of a user who has saved none: every option (tests/fixtures/default-interests.json). */
    val DEFAULT_INTERESTS: List<String> = options.flatMap { (id, children) -> listOf(id) + children.map { "$id/$it" } }

    fun decodeInterests(value: Any?): JsArray {
        if (value === Undefined || value == null) return jsArray(DEFAULT_INTERESTS)
        if (value !is JsArray || value.size > Limits.MAX_INTERESTS ||
            value.any { it !is String || it.isEmpty() || it.length > Limits.MAX_INTEREST_LENGTH } || value.toSet().size != value.size
        ) throw AppError("savedDataCorrupted")
        return jsArray(value)
    }
}
