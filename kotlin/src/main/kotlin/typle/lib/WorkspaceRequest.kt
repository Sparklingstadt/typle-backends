package typle.lib

// src/lib/workspaceRequest.ts and decodeDocument of src/lib/workspaceDocument.ts.

/** A workspace as the app reads it. */
data class WorkspaceDocument(val packs: List<JsObject>, val lists: List<JsObject>, val devices: JsArray, val interests: JsArray) {
    val library get() = Library(packs, lists)
}

/** A change of a save; null parts stay as they are. */
data class WorkspaceChange(
    val lists: List<JsObject>? = null,
    val packs: List<JsObject>? = null,
    val devices: JsArray? = null,
    val interests: JsArray? = null,
)

object WorkspaceRequest {
    /** validateLists: the lists read as saved data, within the limits of a save (PlainError past them). */
    fun validateLists(value: Any?): List<JsObject> {
        val lists = WordLists.decodeLists(value)
        fun strings(v: Any?) = (v as JsArray?)?.map { it as String }
        if (lists.any { list ->
                val words = list["words"] as JsArray
                val tags = strings(list["tags"])
                val levels = strings(list["levels"])
                words.size > Limits.MAX_WORDS || (list["records"] as JsArray).size > Limits.MAX_RECORDS ||
                    (list["name"] as String).length > Limits.MAX_LIST_NAME ||
                    ((list["category"] as String?)?.length ?: 0) > Limits.MAX_CATEGORY || (tags?.size ?: 0) > Limits.MAX_TAGS ||
                    tags?.any { it.length > Limits.MAX_TAG } == true || ((list["group"] as String?)?.length ?: 0) > Limits.MAX_GROUP ||
                    (levels?.size ?: 0) > Limits.MAX_SAVED_LEVELS || levels?.any { it.length > Limits.MAX_LEVEL } == true ||
                    words.any {
                        (prop(it, "input") as String).length > Limits.MAX_WORD_TEXT || (prop(it, "display") as String).length > Limits.MAX_WORD_TEXT ||
                            (prop(it, "annotation") as String).length > Limits.MAX_ANNOTATION
                    }
            }) throw PlainError("Workspace too large")
        return lists
    }

    /** validateWorkspaceRequest: the revision and the change of a body. */
    fun validate(body: JsObject): Pair<Double, WorkspaceChange> {
        val revision = prop(body, "revision")
        if (!isSafeInteger(revision) || (revision as Double) < 0) throw PlainError("Invalid revision")
        if (prop(body, "profileId") !== Undefined || prop(body, "profiles") !== Undefined) throw PlainError("Invalid request")
        if (prop(body, "packs") !== Undefined && prop(body, "lists") === Undefined) throw PlainError("Invalid request")
        val lists = if (prop(body, "lists") === Undefined) null else validateLists(body["lists"])
        val packs = if (prop(body, "packs") === Undefined) null else WordPacks.decodePacks(body["packs"])
        if (lists != null && packs != null) WordPacks.validateLibrary(Library(packs, lists))
        val devices = if (prop(body, "devices") === Undefined) null else Devices.decodeDevices(body["devices"])
        val interests = if (prop(body, "interests") === Undefined) null else Interests.decodeInterests(body["interests"])
        // -0 is revision 0, as Number(-0) compares.
        return (revision + 0.0) to WorkspaceChange(lists, packs, devices, interests)
    }
}

object WorkspaceDocuments {
    /** decodeDocument: packs missing or null means none are saved. */
    fun decode(lists: Any?, packs: Any?, devices: Any?, interests: Any?): WorkspaceDocument {
        val decodedLists = WordLists.decodeLists(lists)
        val decodedDevices = Devices.decodeDevices(if (devices == null || devices === Undefined) JsArray() else devices)
        val decodedInterests = Interests.decodeInterests(interests)
        val decodedPacks = if (packs == null || packs === Undefined) null else WordPacks.decodePacks(packs)
        val library = WordPacks.normalizeLibrary(decodedLists, decodedPacks)
        return WorkspaceDocument(library.packs, library.lists, decodedDevices, decodedInterests)
    }
}
