package typle.lib

// src/lib/followedPacks.ts and src/lib/packs/{index,catalog}.ts: the official packs and keeping followed ones in line.

/** An official pack as the catalog holds it, in the saved shape. */
class OfficialPack(val catalogId: String, val version: Double, val pack: JsObject, val lists: List<JsObject>)

object FollowedPacks {
    fun isFollowed(pack: JsObject) = prop(pack["origin"], "followed") == true

    /** The lists as they are saved: those of a followed pack without their words (withoutFollowedWords). */
    fun withoutFollowedWords(library: Library): Library {
        val followed = library.packs.filter(::isFollowed).map { it["id"] }.toSet()
        return library.copy(lists = library.lists.map { list ->
            if (list["packId"] in followed && isTruthyObject(list["origin"])) list.with("words", JsArray()) else list
        })
    }

    private fun isTruthyObject(value: Any?) = value is JsObject || value is JsArray || (value is String && value.isNotEmpty()) ||
        value == true || (value is Double && value != 0.0 && !value.isNaN())

    /** followOfficial: removes copies from before v0.98.0 and makes each followed pack equal to its official pack. */
    fun followOfficial(library: Library, official: List<OfficialPack>): Library {
        val catalog = LinkedHashMap<String, OfficialPack>()
        for (entry in official) catalog[entry.catalogId] = entry
        val kept = library.packs.filter { pack ->
            val origin = pack["origin"]
            if (isTruthyObject(origin)) isFollowed(pack) && prop(origin, "catalogId") in catalog.keys
            else pack["id"] !in catalog.keys
        }
        val packs = ArrayList<JsObject>()
        val synced = HashMap<String, List<JsObject>>()
        for (pack in kept) {
            if (!isFollowed(pack)) { packs.add(pack); continue }
            val entry = catalog[prop(pack["origin"], "catalogId") as String]!!
            val rest = pack.without("description", "category", "tags")
            val description = entry.pack["description"] as String?
            val category = entry.pack["category"] as String?
            val tags = entry.pack["tags"] as JsArray?
            if (!description.isNullOrEmpty()) rest["description"] = description
            if (!category.isNullOrEmpty()) rest["category"] = category
            if (!tags.isNullOrEmpty()) rest["tags"] = jsArray(tags)
            packs.add(rest)
            val packId = pack["id"] as String
            val mine = library.lists.filter { it["packId"] == packId }
            synced[packId] = entry.lists.map { item ->
                val found = mine.find { prop(it["origin"], "listId") == item["id"] }
                val base = found ?: jsObject(
                    "id" to "$packId:${item["id"]}", "packId" to packId, "records" to JsArray(), "createdAt" to item["createdAt"],
                    "origin" to jsObject("listId" to item["id"]),
                )
                val next = base.without("levels", "writing")
                next["name"] = item["name"]
                val levels = item["levels"] as JsArray?
                if (!levels.isNullOrEmpty()) next["levels"] = jsArray(levels)
                val writing = item["writing"] as JsObject?
                if (writing != null) next["writing"] = writing.copy()
                next["words"] = jsArray((item["words"] as JsArray).map { (it as JsObject).copy() })
                next
            }
        }
        val byId = packs.associateBy { it["id"] as String }
        val placed = HashSet<String>()
        val lists = library.lists.flatMap { list ->
            val pack = byId[list["packId"] as String?] ?: return@flatMap emptyList()
            val id = pack["id"] as String
            if (!synced.containsKey(id)) return@flatMap listOf(list)
            if (!placed.add(id)) return@flatMap emptyList()
            synced[id]!!
        }
        return Library(packs, lists)
    }
}

/** The official packs (src/lib/packs/catalog.ts), built once from the JSON the build copied from src/lib/packs. */
object Catalog {
    val official: List<OfficialPack> by lazy {
        val library = WordPacks.convertLibrary(Resources.officialLists())
        val packs = library.packs.map { it.with("origin", jsObject("catalogId" to it["id"], "version" to 1.0)) }
        packs.map { pack -> OfficialPack(pack["id"] as String, 1.0, pack, library.lists.filter { it["packId"] == pack["id"] }) }
    }

    /** followed of catalog.ts: the library in line with the catalog. */
    fun followed(library: Library): Library = FollowedPacks.followOfficial(library, official)
}

/** The data the build copies from the repository (kotlin/build.gradle.kts, copyAppData). */
object Resources {
    private fun read(path: String): String {
        val stream = Resources::class.java.getResourceAsStream("/app-data/$path") ?: error("missing resource app-data/$path")
        return stream.use { it.readBytes().toString(Charsets.UTF_8) }
    }

    fun defaultDeviceName(): String = prop(JsonParser.parse(read("format.json")), "defaultDevice") as String

    /** initialLists of src/lib/packs/index.ts: samples (numbered), countries, then the commands of manifest.json. */
    fun officialLists(): List<JsObject> {
        val samples = JsonParser.parse(read("samples.json")) as JsArray
        val countries = JsonParser.parse(read("countries.json")) as JsArray
        val manifest = JsonParser.parse(read("commands/manifest.json")) as JsArray
        val sampleLists = samples.mapIndexed { index, list -> (list as JsObject).with("id", "sample-${index + 1}").with("records", JsArray()) }
        val commands = manifest.map { name ->
            val list = JsonParser.parse(read("commands/$name")) as JsObject
            list.with("words", jsArray((list["words"] as JsArray).map { word ->
                val w = word as JsObject
                if (!WordLists.isInputType(w["input_type"])) error("Invalid official input_type")
                w.with("input_type", w["input_type"])
            }))
        }
        @Suppress("UNCHECKED_CAST")
        return sampleLists + (countries as List<JsObject>) + commands
    }
}
