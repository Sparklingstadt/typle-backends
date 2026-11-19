package typle.lib

// src/lib/wordLists.ts and the record checks of src/lib/records.ts that reading lists needs.

object WordLists {
    private val writingLanguages = setOf("en", "ja", "zh-Hans", "zh-Hant", "ko", "fr", "de", "es", "ru", "cs")

    fun isInputType(value: Any?) = value == "command" || value == "code" || value == "word"

    /** A valid mark: an object whose only key is a known language. */
    fun isWritingMark(value: Any?): Boolean =
        value is JsObject && value.size == 1 && prop(value, "language").let { it is String && it in writingLanguages }

    fun validateList(list: JsObject) {
        val name = list["name"] as String
        if (name.jsTrim().isEmpty()) throw AppError("listNameRequired")
        val words = list["words"] as JsArray
        if (words.isEmpty()) throw AppError("wordsRequired")
        if (words.any { (prop(it, "display") as String).jsTrim().isEmpty() || (prop(it, "input") as String).jsTrim().isEmpty() }) {
            throw AppError("wordFieldsRequired")
        }
        if (words.any { prop(it, "input_type") !== Undefined && !isInputType(prop(it, "input_type")) }) throw AppError("savedDataCorrupted")
        if (prop(list, "writing") !== Undefined && !isWritingMark(prop(list, "writing"))) throw AppError("invalidWritingLanguage")
    }

    private fun dropNulls(value: JsObject, names: List<String>) {
        for (name in names) if (value.containsKey(name) && value[name] == null) value.remove(name)
    }

    private fun finite(value: Any?) = value is Double && value.isFinite()

    private fun badRecord(record: Any?): Boolean {
        if (record == null || record == false || record == "" || (record is Double && (record == 0.0 || record.isNaN()))) return true
        val time = prop(record, "time")
        val date = prop(record, "date")
        val device = prop(record, "device")
        val mode = prop(record, "mode")
        val id = prop(record, "id")
        val accuracy = prop(record, "accuracy")
        val mistakes = prop(record, "mistakes")
        val wordCount = prop(record, "wordCount")
        val missed = prop(record, "missed")
        return !finite(time) || (time as Double) < 0 || date !is String || !JsDate.parse(date).isFinite() ||
            (device !== Undefined && !Devices.isDevice(device)) ||
            (mode !== Undefined && mode != "recall" && mode != "writing") ||
            (id !== Undefined && id !is String) ||
            (accuracy !== Undefined && (!finite(accuracy) || (accuracy as Double) < 0 || accuracy > 100)) ||
            (mistakes !== Undefined && (!isInteger(mistakes) || (mistakes as Double) < 0)) ||
            (wordCount !== Undefined && (!isInteger(wordCount) || (wordCount as Double) < 1)) ||
            (missed !== Undefined && !Records.isMissedWords(missed))
    }

    private fun badWord(word: Any?): Boolean {
        if (word == null || word == false || word == "" || (word is Double && (word == 0.0 || word.isNaN()))) return true
        return prop(word, "display") !is String || prop(word, "input") !is String || prop(word, "annotation") !is String ||
            (prop(word, "input_type") !== Undefined && !isInputType(prop(word, "input_type")))
    }

    /** decodeLists(JSON.stringify({ version: 1, lists: value })): the lists checked, as copies. */
    fun decodeLists(value: Any?): List<JsObject> {
        if (value === Undefined) throw AppError("savedDataUnreadable")
        val data = deepCopy(value)
        if (data !is JsArray) throw AppError("savedDataUnreadable")
        for (list in data) {
            if (list !is JsObject) continue
            dropNulls(list, listOf("writing", "tags", "category", "group", "levels", "packId"))
            val records = list["records"]
            if (records is JsArray) {
                for (record in records) if (record is JsObject) dropNulls(record, listOf("id", "mistakes", "accuracy", "wordCount", "device", "missed"))
            }
        }
        val ids = HashSet<String>()
        for (list in data) {
            val id = prop(list, "id")
            val name = prop(list, "name")
            val words = prop(list, "words")
            val records = prop(list, "records")
            val category = prop(list, "category")
            val tags = prop(list, "tags")
            val group = prop(list, "group")
            val packId = prop(list, "packId")
            val writing = prop(list, "writing")
            val levels = prop(list, "levels")
            if (list !is JsObject || id !is String || id.isEmpty() || id in ids || name !is String ||
                prop(list, "createdAt") !is String || words !is JsArray || records !is JsArray ||
                (category !== Undefined && category !is String) ||
                (tags !== Undefined && (tags !is JsArray || tags.any { it !is String })) ||
                (group !== Undefined && group !is String) ||
                (packId !== Undefined && packId !is String) ||
                (writing !== Undefined && !isWritingMark(writing)) ||
                (levels !== Undefined && (levels !is JsArray || levels.any { it !is String })) ||
                words.any(::badWord) || records.any(::badRecord)
            ) throw AppError("savedDataCorrupted")
            // A list of a followed pack (with an origin) is saved without its words, which the catalog gives back.
            if (words.isNotEmpty() || prop(prop(list, "origin"), "listId") !is String) validateList(list)
            else if (name.jsTrim().isEmpty()) throw AppError("listNameRequired")
            ids.add(id)
        }
        // Fields of link sharing are dropped, and an origin or weak review of another shape.
        return data.map { item ->
            val list = item as JsObject
            val origin = list["origin"]
            val weakReview = if (list.containsKey("weakReview")) list["weakReview"] else Undefined
            val hasOrigin = list.containsKey("origin")
            val result = list.without("shared", "shareId", "origin", "weakReview")
            if (hasOrigin && prop(origin, "listId") is String) result["origin"] = origin
            if (Records.isWeakReview(weakReview)) result["weakReview"] = weakReview
            result
        }
    }
}

/** src/lib/records.ts: the checks and changes of a list's practice records. */
object Records {
    /** The built-in device's name in the app's base language (src/messages/ja/format.json defaultDevice). */
    val DEFAULT_DEVICE_NAME: String by lazy { Resources.defaultDeviceName() }

    fun isMissedWords(value: Any?, limit: Int = Limits.MAX_MISSED_WORDS): Boolean =
        value is JsArray && value.size <= limit && value.all { item ->
            isObjectLike(item) && (item !is JsObject || item.keys.all { it == "word" || it == "count" }) &&
                prop(item, "word").let { it is String && it.isNotEmpty() && it.length <= Limits.MAX_WORD_TEXT } &&
                prop(item, "count").let { isSafeInteger(it) && (it as Double) >= 1 }
        }

    fun isWeakReview(value: Any?): Boolean {
        if (value !is JsObject) return false
        val date = prop(value, "date")
        val missed = prop(value, "missed")
        return value.keys.all { it == "date" || it == "missed" } && date is String && JsDate.parse(date).isFinite() &&
            (missed === Undefined || isMissedWords(missed, Limits.MAX_WEAK_REVIEW_MISSED))
    }

    private fun finite(value: Any?) = value is Double && value.isFinite()
    private fun integer(value: Any?, min: Int) = finite(value) && isSafeInteger(value) && (value as Double) >= min
    private val recordId = Regex("^[A-Za-z0-9_-]+$")

    class Added(val lists: List<JsObject>, val record: Any?, val added: Boolean)

    /**
     * Adds a run to a list (addRecord). A run with the id of one the list already has is not added again: that saved
     * record is returned with added false.
     */
    fun addRecord(lists: List<JsObject>, devices: List<Any?>, listId: String, input: JsObject): Added {
        val list = lists.find { it["id"] == listId } ?: throw AppError("recordListDeleted")
        val dateText = prop(input, "date")
        val time = if (dateText is String) JsDate.parse(dateText) else Double.NaN
        val year = if (dateText is String) JsDate.utcYear(time) else Double.NaN
        val run = prop(input, "time")
        val accuracy = prop(input, "accuracy")
        val mistakes = prop(input, "mistakes")
        val wordCount = prop(input, "wordCount")
        val mode = prop(input, "mode")
        val id = prop(input, "id")
        val missed = prop(input, "missed")
        if (!finite(run) || (run as Double) < 0 || !finite(accuracy) || (accuracy as Double) < 0 || accuracy > 100 ||
            !(year >= 0 && year <= 9999) || !integer(mistakes, 0) || (wordCount !== Undefined && !integer(wordCount, 1)) ||
            (mode !== Undefined && mode != "recall" && mode != "writing") || id !is String || id.length > Limits.MAX_RECORD_ID ||
            !recordId.matches(id) || (missed !== Undefined && missed != null && !isMissedWords(missed))
        ) throw AppError("invalidRecord")
        val records = list["records"] as JsArray
        val saved = records.find { prop(it, "id") == id }
        if (saved != null) return Added(lists, saved, false)
        if (records.size >= Limits.MAX_RECORDS) throw AppError("recordLimit")
        var device: JsObject? = null
        val sentDevice = prop(input, "device")
        val deviceId = prop(input, "deviceId")
        if (sentDevice !== Undefined) {
            if (deviceId !== Undefined || !Devices.isDevice(sentDevice)) throw AppError("invalidRecord")
            device = jsObject("id" to prop(sentDevice, "id"), "name" to prop(sentDevice, "name"), "layout" to prop(sentDevice, "layout"))
        } else if (deviceId !== Undefined) {
            device = if (deviceId == Devices.DEFAULT_DEVICE_ID) Devices.defaultDevice(DEFAULT_DEVICE_NAME)
            else devices.find { prop(it, "id") == deviceId } as JsObject?
            if (device == null) throw AppError("deviceNotFound")
        }
        val record = jsObject(
            "id" to id, "time" to run, "date" to JsDate.toIsoString(time), "mistakes" to mistakes, "accuracy" to accuracy,
            "wordCount" to (if (wordCount === Undefined) (list["words"] as JsArray).size.toDouble() else wordCount),
        )
        if (mode !== Undefined) record["mode"] = mode
        if (device != null) record["device"] = device.copy()
        if (missed is JsArray && missed.isNotEmpty()) {
            record["missed"] = jsArray(missed.map { jsObject("word" to prop(it, "word"), "count" to prop(it, "count")) })
        }
        val next = lists.map { if (it === list) it.with("records", jsArray(records + record)) else it }
        return Added(next, record, true)
    }

    /** Deletes a run from a list (deleteRecord); null when it, or its list, is already gone. */
    fun deleteRecord(lists: List<JsObject>, listId: String, recordId: String): List<JsObject>? {
        val list = lists.find { it["id"] == listId }
        val records = list?.get("records") as JsArray?
        if (recordId.isEmpty() || records == null || records.none { prop(it, "id") == recordId }) return null
        return lists.map { if (it === list) it.with("records", jsArray(records.filter { r -> prop(r, "id") != recordId })) else it }
    }
}
