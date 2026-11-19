package typle.lib

// src/lib/wordPacks.ts, src/lib/workspaceDocument.ts (unique, fit) and the counts of src/lib/listOverview.ts.

/** A library: packs and lists in the saved shape, each an object as saved (unknown fields kept, in their order). */
data class Library(val packs: List<JsObject>, val lists: List<JsObject>)

object WordPacks {
    fun nameKey(name: String) = name.jsTrim().jsLower()

    /** unique of workspaceDocument.ts: wanted, or its first variant whose lowercase is not taken yet. */
    fun unique(taken: MutableSet<String>, wanted: String, variant: (Int) -> String): String {
        var value = wanted
        var count = 2
        while (value.jsLower() in taken) value = variant(count++)
        taken.add(value.jsLower())
        return value
    }

    /** fit of workspaceDocument.ts: name cut to leave room for suffix, without splitting a surrogate pair. */
    fun fit(name: String, suffix: String, limit: Int = Limits.MAX_NAME): String {
        val cut = name.jsSliceTo(limit - suffix.length)
        val kept = if (cut.isNotEmpty() && Character.isHighSurrogate(cut.last())) cut.substring(0, cut.length - 1) else cut
        return kept.jsTrimEnd() + suffix
    }

    private fun numbered(name: String): (Int) -> String = { count -> fit(name, "（$count）") }

    private fun levelsOf(list: JsObject): List<String>? = (list["levels"] as JsArray?)?.map { it as String }

    fun levelPath(list: JsObject): List<String> = levelsOf(list)?.takeIf { it.isNotEmpty() } ?: listOf(list["name"] as String)

    fun pathsOverlap(a: List<String>, b: List<String>): Boolean {
        val (short, long) = if (a.size <= b.size) a to b else b to a
        return short.indices.all { short[it] == long[it] }
    }

    private fun bare(level: String): String {
        val isEdge = { c: Char -> isJsSpace(c) || c == '・' }
        return level.trim(isEdge)
    }

    private fun joinLevels(levels: List<String>): String? =
        bare(fit(levels.map(::bare).filter { it.isNotEmpty() }.joinToString("・"), "", Limits.MAX_LEVEL)).ifEmpty { null }

    fun collapseLevels(list: JsObject): JsObject {
        val levels = levelsOf(list) ?: return list
        if (levels.size <= 1) return list
        val joined = joinLevels(levels)
        return if (joined != null) list.with("levels", jsArray(listOf(joined))) else list.without("levels")
    }

    fun separateLevels(library: Library): Library {
        val lists = library.lists.map(::collapseLevels)
        val names = HashMap<String?, MutableSet<String>>()
        for (list in lists) names.getOrPut(list["packId"] as String?) { HashSet() }.add(nameKey(list["name"] as String))
        val accepted = HashMap<String?, MutableList<List<String>>>()
        return library.copy(lists = lists.map { list ->
            val packId = list["packId"] as String?
            val paths = accepted.getOrPut(packId) { ArrayList() }
            val taken = names[packId]!!
            val hasLevels = !levelsOf(list).isNullOrEmpty()
            val base = levelPath(list)
            val path = base.toMutableList()
            val name = list["name"] as String
            val nameTaken = { !hasLevels && nameKey(path[0]) != nameKey(name) && nameKey(path[0]) in taken }
            var count = 2
            while (paths.any { pathsOverlap(it, path) } || nameTaken()) {
                path[0] = fit(base[0], "（$count）", if (hasLevels) Limits.MAX_LEVEL else Limits.MAX_LIST_NAME)
                count++
            }
            paths.add(path)
            if (path[0] == base[0]) list
            else if (hasLevels) list.with("levels", jsArray(path))
            else {
                taken.add(nameKey(path[0]))
                list.with("name", path[0])
            }
        })
    }

    /** A pack with description, tags and origin.followed saved as null left out (withoutNulls). */
    private fun withoutNulls(pack: Any?): Any? {
        if (pack !is JsObject) return pack
        val description = prop(pack, "description")
        val tags = prop(pack, "tags")
        val origin = prop(pack, "origin")
        val cleanOrigin = if (isObjectLike(origin) && prop(origin, "followed").let { it == null && it !== Undefined } && origin is JsObject)
            origin.without("followed") else origin
        val result = pack.without("description", "tags", "origin")
        if (description != null && description !== Undefined) result["description"] = description
        if (tags != null && tags !== Undefined) result["tags"] = tags
        if (origin !== Undefined) result["origin"] = cleanOrigin
        return result
    }

    private fun truthy(value: Any?) = !(value == null || value === Undefined || value == false || value == "" ||
        (value is Double && (value == 0.0 || value.isNaN())))

    fun decodePacks(value: Any?): List<JsObject> {
        if (value !is JsArray) throw AppError("savedDataCorrupted")
        val packs = value.map(::withoutNulls)
        val ids = HashSet<String>()
        for (pack in packs) {
            val origin = prop(pack, "origin")
            val id = prop(pack, "id")
            val tags = prop(pack, "tags")
            if (!truthy(pack) || pack !is JsObject || id !is String || id.isEmpty() || id in ids || prop(pack, "name") !is String ||
                prop(pack, "createdAt") !is String ||
                (prop(pack, "description") !== Undefined && prop(pack, "description") !is String) ||
                (prop(pack, "category") !== Undefined && prop(pack, "category") !is String) ||
                (tags !== Undefined && (tags !is JsArray || tags.any { it !is String })) ||
                (origin !== Undefined && (!truthy(origin) || prop(origin, "catalogId") !is String || prop(origin, "version") !is Double ||
                    (prop(origin, "followed") !== Undefined && prop(origin, "followed") != true)))
            ) throw AppError("savedDataCorrupted")
            ids.add(id)
        }
        @Suppress("UNCHECKED_CAST")
        return packs as List<JsObject>
    }

    /** normalizeLabels of wordLists.ts: category and tags trimmed, empty and repeated tags dropped, absent when none. */
    fun normalizeLabels(category: String?, tags: List<String>?): JsObject {
        val name = category?.jsTrim() ?: ""
        val seen = HashSet<String>()
        val kept = ArrayList<String>()
        for (tag in tags ?: emptyList()) {
            val value = tag.jsTrim()
            if (value.isEmpty() || value.jsLower() in seen) continue
            seen.add(value.jsLower())
            kept.add(value)
        }
        val result = JsObject()
        if (name.isNotEmpty()) result["category"] = name
        if (kept.isNotEmpty()) result["tags"] = jsArray(kept)
        return result
    }

    private fun stringsOf(value: Any?): List<String>? = (value as? JsArray)?.map { it as String }

    private fun withoutLabels(list: JsObject) = list.without("group", "category", "tags")

    private fun labelsOf(category: String?, tags: List<String>): JsObject {
        val labels = normalizeLabels(category, tags)
        val kept = labels["tags"] as JsArray? ?: return labels
        return labels.with("tags", jsArray(kept.take(Limits.MAX_TAGS)))
    }

    /** Makes packs of lists written with groups (convertLibrary), as the official lists are made into packs. */
    fun convertLibrary(lists: List<JsObject>): Library {
        val packs = ArrayList<JsObject>()
        val packOf = HashMap<String, JsObject>()
        val names = HashSet<String>()
        val packIds = lists.map { list ->
            val group = (list["group"] as String?)?.jsTrim()
            val key = if (!group.isNullOrEmpty()) "group:$group" else "list:${list["id"]}"
            packOf[key]?.let { return@map it["id"] as String }
            val members = if (!group.isNullOrEmpty()) lists.filter { (it["group"] as String?)?.jsTrim() == group } else listOf(list)
            val oldest = members.reduce { a, b ->
                if (JsDate.parse(b["createdAt"] as String) < JsDate.parse(a["createdAt"] as String)) b else a
            }
            val wanted = if (!group.isNullOrEmpty()) group else (list["name"] as String).jsTrim()
            val pack = jsObject("id" to "pack-${list["id"]}", "name" to unique(names, wanted, numbered(wanted)))
            pack.putAll(labelsOf(list["category"] as String?, members.flatMap { stringsOf(it["tags"]) ?: emptyList() }))
            pack["createdAt"] = oldest["createdAt"]
            packOf[key] = pack
            packs.add(pack)
            pack["id"] as String
        }
        return separateLevels(Library(packs, lists.mapIndexed { i, list -> withoutLabels(list).with("packId", packIds[i]) }))
    }

    /** Keeps the rules of ownership for data an older app saved: every list in one pack, no empty packs, no repeated names. */
    fun repairLibrary(library: Library): Library {
        val packs = library.packs.map { it.copy() }.toMutableList()
        val byId = LinkedHashMap<String, JsObject>()
        for (pack in packs) byId[pack["id"] as String] = pack
        val placed = library.lists.map { list ->
            val packId = list["packId"]
            if (packId is String && byId.containsKey(packId)) return@map list
            val id = "pack-${list["id"]}"
            if (!byId.containsKey(id)) {
                val pack = jsObject("id" to id, "name" to (list["name"] as String).jsTrim(), "createdAt" to list["createdAt"])
                byId[id] = pack
                packs.add(pack)
            }
            list.with("packId", id)
        }
        // Labels an older app left on a list move to the pack only while the pack has none.
        val lists = placed.map { list ->
            val pack = byId[list["packId"] as String]!!
            val packTags = pack["tags"] as JsArray?
            if (!truthy(pack["category"]) && (packTags == null || packTags.isEmpty())) {
                pack.putAll(normalizeLabels(list["category"] as String?, stringsOf(list["tags"])))
            }
            withoutLabels(list)
        }
        val used = lists.map { it["packId"] }.toSet()
        val names = HashSet<String>()
        val kept = packs.filter { it["id"] in used }.map { pack ->
            val name = (pack["name"] as String).jsTrim()
            pack.with("name", unique(names, name, numbered(name)))
        }
        val listNames = HashMap<String, MutableSet<String>>()
        return separateLevels(Library(kept, lists.map { list ->
            val taken = listNames.getOrPut(list["packId"] as String) { HashSet() }
            val name = (list["name"] as String).jsTrim()
            list.with("name", unique(taken, name, numbered(name)))
        }))
    }

    fun normalizeLibrary(lists: List<JsObject>, packs: List<JsObject>?): Library = repairLibrary(Library(packs ?: emptyList(), lists))

    /** Refuses a library that is not in the saved shape (PlainError), or whose levels a save may not have (AppError). */
    fun validateLibrary(library: Library) {
        val invalid = { PlainError("Invalid packs") }
        val ids = HashSet<String>()
        val names = HashSet<String>()
        for (pack in library.packs) {
            val packName = pack["name"] as String
            val name = nameKey(packName)
            val id = pack["id"] as String
            val tags = stringsOf(pack["tags"])
            if (id.isEmpty() || id in ids || name.isEmpty() || packName.length > Limits.MAX_PACK_NAME || name in names ||
                ((pack["description"] as String?)?.length ?: 0) > Limits.MAX_PACK_DESCRIPTION ||
                ((pack["category"] as String?)?.length ?: 0) > Limits.MAX_CATEGORY ||
                (tags?.size ?: 0) > Limits.MAX_TAGS || tags?.any { it.length > Limits.MAX_TAG } == true
            ) throw invalid()
            ids.add(id)
            names.add(name)
        }
        val listNames = HashMap<String, MutableSet<String>>()
        for (list in library.lists) {
            val packId = list["packId"] as String?
            val taken = listNames.getOrPut(packId ?: "") { HashSet() }
            val name = nameKey(list["name"] as String)
            if (packId.isNullOrEmpty() || packId !in ids || name in taken ||
                list.containsKey("group") || list.containsKey("category") || list.containsKey("tags")
            ) throw invalid()
            taken.add(name)
        }
        if (ids.size != listNames.size) throw invalid()
        val paths = HashMap<String, MutableList<List<String>>>()
        for (list in library.lists) {
            if ((levelsOf(list)?.size ?: 0) > Limits.MAX_LEVELS) throw AppError("tooManyLevels")
            val mine = paths.getOrPut(list["packId"] as String) { ArrayList() }
            if (mine.any { pathsOverlap(it, levelPath(list)) }) throw AppError("levelsConflict")
            mine.add(levelPath(list))
        }
    }

    // The counts of src/lib/listOverview.ts.
    private fun followedIds(packs: List<JsObject>) = packs.filter { prop(it["origin"], "followed") == true }.map { it["id"] }.toSet()

    fun ownListCount(library: Library): Int {
        val followed = followedIds(library.packs)
        return library.lists.count { it["packId"] !in followed }
    }

    fun followedPackCount(packs: List<JsObject>) = followedIds(packs).size
}
