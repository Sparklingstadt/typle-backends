package typle.server

import typle.lib.AppError
import typle.lib.Catalog
import typle.lib.FollowedPacks
import typle.lib.Interests
import typle.lib.JsArray
import typle.lib.JsObject
import typle.lib.JsonParser
import typle.lib.JsonWriter
import typle.lib.Library
import typle.lib.Limits
import typle.lib.Records
import typle.lib.Undefined
import typle.lib.WordPacks
import typle.lib.WorkspaceChange
import typle.lib.WorkspaceDocument
import typle.lib.WorkspaceDocuments
import typle.lib.WorkspaceRequest
import typle.lib.jsArray
import typle.lib.jsObject
import typle.lib.prop
import typle.lib.copy
import typle.lib.with
import typle.lib.without
import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction
import java.sql.Connection

// src/server/workspace.ts: /api/workspace and /api/workspace/records. Saves are not measured (typle_save_metrics).

/** A workspace as saved: the values of its jsonb columns and its revision. */
class SavedWorkspace(val lists: JsArray, val packs: Any?, val devices: Any?, val interests: Any?, val revision: Int)

/** A save named a revision the workspace has moved on from (409). */
class Conflict : Exception("conflict")

/** The parts a save writes (encodeChange). */
class EncodedLibrary(val lists: List<JsObject>, val packs: List<JsObject>, val rows: List<RecordRow>, val own: Int, val followed: Int, val size: Long)
class Encoded(val devices: JsArray?, val interests: JsArray?, val library: EncodedLibrary?)

object Workspace {
    private const val MAX_LISTS_BYTES = 896 shl 10
    private const val MAX_BODY_BYTES = 1 shl 20

    /** The UTF-8 length of text as Buffer.byteLength counts it (a lone surrogate is 3 bytes). */
    fun utf8Length(text: String): Long {
        var length = 0L
        var i = 0
        while (i < text.length) {
            val c = text[i]
            length += when {
                c.code < 0x80 -> 1
                c.code < 0x800 -> 2
                Character.isHighSurrogate(c) && i + 1 < text.length && Character.isLowSurrogate(text[i + 1]) -> { i++; 4 }
                else -> 3
            }
            i++
        }
        return length
    }

    private fun parseJson(text: String?): Any? = if (text == null) null else JsonParser.parse(text)

    /** The owner's saved workspace, or null; checked as the app reads it, answered as saved. */
    fun readSavedWorkspace(connection: Connection, owner: String): SavedWorkspace? {
        val row = connection.query(
            """SELECT lists::text AS lists, packs::text AS packs, devices::text AS devices, interests::text AS interests, revision,
      CASE WHEN records_split THEN (SELECT COALESCE(jsonb_agg(jsonb_build_array(r.list_id, r.data) ORDER BY r.list_id, r.position), '[]'::jsonb)
        FROM typle_records r WHERE r.owner_id = typle_workspaces.owner_id)::text END AS records
    FROM typle_workspaces WHERE owner_id = ?""",
            owner,
        ) { rows ->
            if (!rows.next()) null
            else listOf(rows.getString(1), rows.getString(2), rows.getString(3), rows.getString(4), rows.getInt(5), rows.getString(6))
        } ?: return null
        var lists = parseJson(row[0] as String) as JsArray
        val records = row[5] as String?
        if (records != null) {
            val stored = LinkedHashMap<String, MutableList<Any?>>()
            for (pair in parseJson(records) as JsArray) {
                val (listId, record) = pair as JsArray
                stored.getOrPut(listId as String) { ArrayList() }.add(record)
            }
            lists = RecordStore.withRecords(lists, stored)
        }
        val saved = SavedWorkspace(lists, parseJson(row[1] as String?), parseJson(row[2] as String), parseJson(row[3] as String?), row[4] as Int)
        // Throws on saved data the app could not read either.
        val document = WorkspaceDocuments.decode(saved.lists, saved.packs, saved.devices, saved.interests ?: Undefined)
        Catalog.followed(document.library)
        return saved
    }

    /** The answer of GET and PUT /api/workspace; revision 0 and no lists when nothing is saved. */
    fun answer(saved: SavedWorkspace?, owner: String): JsObject = jsObject(
        "lists" to (saved?.lists ?: JsArray()),
        "packs" to saved?.packs,
        "devices" to (saved?.devices ?: JsArray()),
        "interests" to (saved?.interests ?: jsArray(Interests.DEFAULT_INTERESTS)),
        "revision" to (saved?.revision ?: 0),
        "accountId" to owner,
    )

    private val loneSurrogateOrNul = { text: String ->
        var bad = false
        var i = 0
        while (i < text.length && !bad) {
            val c = text[i]
            if (c == '\u0000') bad = true
            else if (Character.isHighSurrogate(c)) {
                if (i + 1 < text.length && Character.isLowSurrogate(text[i + 1])) i++ else bad = true
            } else if (Character.isLowSurrogate(c)) bad = true
            i++
        }
        bad
    }

    /** Whether value holds a string (or key) jsonb cannot keep: one with U+0000 or a lone surrogate. */
    fun unstorable(value: Any?): Boolean = when (value) {
        is String -> loneSurrogateOrNul(value)
        is JsArray -> value.any(::unstorable)
        is JsObject -> value.entries.any { (k, v) -> unstorable(k) || unstorable(v) }
        else -> false
    }

    sealed interface ChangeRead
    class ChangeOk(val revision: Long, val change: WorkspaceChange) : ChangeRead
    class ChangeRefused(val key: String) : ChangeRead

    /** The revision and change of a PUT body, answered as the Go backend answered it (readWorkspaceChange). */
    fun readWorkspaceChange(fields: JsObject): ChangeRead {
        val invalid = ChangeRefused("invalidWorkspace")
        for (name in listOf("lists", "packs", "devices")) if (fields.containsKey(name) && fields[name] == null) return invalid
        if (listOf("lists", "packs", "devices", "interests").any { fields.containsKey(it) && unstorable(fields[it]) }) return invalid
        val interests = prop(fields, "interests")
        val rest = fields.without("accountId", "interests")
        val severalLevels = HashMap<Int, Any?>()
        val restLists = prop(rest, "lists")
        val lists: Any? = if (restLists is JsArray && prop(rest, "packs") !== Undefined) {
            jsArray(restLists.mapIndexed { index, item ->
                val levels = prop(item, "levels")
                if (levels !is JsArray || levels.size <= 1 || levels.size > Limits.MAX_SAVED_LEVELS ||
                    levels.any { it !is String || it.length > Limits.MAX_LEVEL }
                ) return@mapIndexed item
                severalLevels[index] = levels
                (item as JsObject).with("levels", jsArray(listOf("\u0000several-$index")))
            })
        } else restLists
        val body = rest.copy()
        if (lists !== Undefined) body["lists"] = lists
        if (interests !== Undefined && interests != null) body["interests"] = interests
        val (revision, checked) = try {
            WorkspaceRequest.validate(body)
        } catch (error: StackOverflowError) {
            return invalid
        } catch (error: Exception) {
            return if (error is AppError && error.key == "levelsConflict") ChangeRefused("levelsConflict") else invalid
        }
        var change = checked
        if (change.lists != null && severalLevels.isNotEmpty()) {
            change = change.copy(lists = change.lists!!.mapIndexed { index, list ->
                if (severalLevels.containsKey(index)) list.with("levels", severalLevels[index]) else list
            })
        }
        if (fields.containsKey("interests") && interests == null) change = change.copy(interests = JsArray())
        return ChangeOk(revision.toLong(), change)
    }


    /** encodeChange: the repaired lists and packs a save writes, without followed words and records, and their rows. */
    fun encodeChange(change: WorkspaceChange): Encoded {
        val devices = change.devices
        val interests = change.interests
        val changeLists = change.lists ?: return Encoded(devices, interests, null)
        val repaired = Catalog.followed(WordPacks.repairLibrary(Library(change.packs ?: emptyList(), changeLists)))
        val rows = RecordStore.recordRows(repaired.lists)
        val slim = FollowedPacks.withoutFollowedWords(repaired)
        // A change the app could not read back is never written.
        try {
            val document = WorkspaceDocuments.decode(jsArray(slim.lists), jsArray(slim.packs), devices ?: JsArray(), interests ?: Undefined)
            Catalog.followed(document.library)
        } catch (error: Exception) {
            throw AppError("invalidWorkspace")
        }
        return Encoded(
            devices, interests,
            EncodedLibrary(
                lists = slim.lists.map { it.with("records", JsArray()) },
                packs = slim.packs,
                rows = rows,
                own = WordPacks.ownListCount(repaired),
                followed = WordPacks.followedPackCount(repaired.packs),
                size = utf8Length(JsonWriter.stringify(jsArray(slim.lists))) + utf8Length(JsonWriter.stringify(jsArray(slim.packs))),
            ),
        )
    }

    class Modified<T>(val result: T, val saved: SavedWorkspace?)
    class Change<T>(val document: WorkspaceDocument, val changed: Boolean, val result: T)

    /** modifyWorkspace: changes the owner's workspace in one transaction that holds the row. */
    fun <T> modifyWorkspace(db: Database, owner: String, change: (WorkspaceDocument) -> Change<T>): Modified<T> = db.transaction { connection ->
        val row = connection.query(
            "SELECT lists::text AS lists, packs::text AS packs, devices::text AS devices, records_split AS split FROM typle_workspaces WHERE owner_id = ? FOR UPDATE",
            owner,
        ) { rows -> if (rows.next()) listOf(rows.getString(1), rows.getString(2), rows.getString(3), rows.getBoolean(4)) else null }
        var document = WorkspaceDocument(emptyList(), emptyList(), JsArray(), JsArray())
        if (row != null) {
            var lists: Any? = parseJson(row[0] as String)
            if (row[3] as Boolean) {
                val records = LinkedHashMap<String, MutableList<Any?>>()
                connection.query("SELECT list_id, data::text AS data FROM typle_records WHERE owner_id = ? ORDER BY list_id, position", owner) { rows ->
                    while (rows.next()) records.getOrPut(rows.getString(1)) { ArrayList() }.add(parseJson(rows.getString(2)))
                }
                lists = RecordStore.withRecords(lists as JsArray, records)
            }
            val decoded = WorkspaceDocuments.decode(lists, parseJson(row[1] as String?), parseJson(row[2] as String), Undefined)
            val followed = Catalog.followed(decoded.library)
            document = decoded.copy(packs = followed.packs, lists = followed.lists)
        }
        val before = encodeChange(WorkspaceChange(lists = document.lists, packs = document.packs)).library!!.size
        val outcome = change(document)
        if (outcome.changed) {
            if (row == null) throw IllegalStateException("no workspace of $owner to change")
            val next = outcome.document
            val library = encodeChange(WorkspaceChange(lists = next.lists, packs = next.packs, devices = next.devices)).library!!
            if (library.size > MAX_LISTS_BYTES && library.size > before) throw AppError("workspaceTooLarge")
            connection.execute(
                """UPDATE typle_workspaces SET lists = ?::jsonb, packs = ?::jsonb, devices = ?::jsonb,
          records_split = true, revision = revision + 1, updated_at = now() WHERE owner_id = ?""",
                JsonWriter.stringify(jsArray(library.lists)), JsonWriter.stringify(jsArray(library.packs)), JsonWriter.stringify(next.devices), owner,
            )
            RecordStore.syncRecords(connection, owner, library.rows)
        }
        Modified(outcome.result, readSavedWorkspace(connection, owner))
    }

    private fun json(value: Any?): String? = if (value == null) null else JsonWriter.stringify(value)

    /** writeWorkspace: saves change over the owner's workspace at revision (0 creates the row), else Conflict. */
    fun writeWorkspace(db: Database, owner: String, revision: Long, change: WorkspaceChange): SavedWorkspace {
        val encoded = encodeChange(change)
        val library = encoded.library
        return db.transaction { connection ->
            if (revision == 0L) {
                if (library != null && library.own > Limits.MAX_LISTS) throw AppError("listLimit")
                if (library != null && library.followed > Limits.MAX_FOLLOWED_PACKS) throw AppError("followLimit")
                // Two first saves of the same owner wait here for each other.
                connection.query("SELECT pg_advisory_xact_lock(hashtext('typle_workspaces:' || ?))", owner) { it.next() }
                val inserted = connection.query(
                    """INSERT INTO typle_workspaces (owner_id, lists, packs, devices, interests, records_split)
        VALUES (?, ?::jsonb, ?::jsonb, ?::jsonb, ?::jsonb, true) ON CONFLICT (owner_id) DO NOTHING RETURNING revision""",
                    owner, json(library?.lists?.let(::jsArray) ?: JsArray()), json(library?.packs?.let(::jsArray)),
                    json(encoded.devices ?: JsArray()), json(encoded.interests),
                ) { it.next() }
                if (!inserted) throw Conflict()
                connection.execute("DELETE FROM typle_records WHERE owner_id = ?", owner)
            } else {
                // The saved lists and followed packs are counted as ownListCount and followedPackCount of
                // src/lib/listOverview.ts count them (typle-r v2.62.0 counts the saved ones with those functions, not in
                // SQL): a pack is followed when its origin's followed is the JSON true, and the followed packs are a set
                // of ids.
                val saved = connection.query(
                    """WITH followed AS (
          SELECT p->>'id' AS id FROM typle_workspaces, jsonb_array_elements(COALESCE(packs, '[]'::jsonb)) AS p
          WHERE owner_id = ? AND p->'origin'->'followed' = 'true'::jsonb)
        SELECT revision,
          (SELECT count(*) FROM jsonb_array_elements(lists) AS l
            WHERE l->>'packId' IS NULL OR l->>'packId' NOT IN (SELECT id FROM followed)) AS own,
          (SELECT count(DISTINCT id) FROM followed) AS followed
        FROM typle_workspaces WHERE owner_id = ?""",
                    owner, owner,
                ) { rows -> if (rows.next()) Triple(rows.getInt(1).toLong(), rows.getLong(2), rows.getLong(3)) else null }
                if (saved == null || saved.first != revision) throw Conflict()
                if (library != null && library.own > Limits.MAX_LISTS && library.own > saved.second) throw AppError("listLimit")
                if (library != null && library.followed > Limits.MAX_FOLLOWED_PACKS && library.followed > saved.third) throw AppError("followLimit")
                // Matching the revision in the UPDATE itself makes the comparison and the write one atomic operation.
                val listsJson = json(library?.lists?.let(::jsArray))
                val updated = connection.query(
                    """UPDATE typle_workspaces SET lists = COALESCE(?::jsonb, lists),
          packs = COALESCE(?::jsonb, packs), devices = COALESCE(?::jsonb, devices), interests = COALESCE(?::jsonb, interests),
          records_split = records_split OR ?::jsonb IS NOT NULL, revision = revision + 1, updated_at = now()
        WHERE owner_id = ? AND revision = ? RETURNING revision""",
                    listsJson, json(library?.packs?.let(::jsArray)), json(encoded.devices), json(encoded.interests), listsJson,
                    owner, revision,
                ) { it.next() }
                if (!updated) throw Conflict()
            }
            if (library != null) RecordStore.syncRecords(connection, owner, library.rows)
            readSavedWorkspace(connection, owner)!!
        }
    }

    /** The body as new TextDecoder().decode reads it: UTF-8, a leading BOM dropped, malformed bytes as U+FFFD. */
    fun decodeBody(bytes: ByteArray): String {
        val start = if (bytes.size >= 3 && bytes[0] == 0xEF.toByte() && bytes[1] == 0xBB.toByte() && bytes[2] == 0xBF.toByte()) 3 else 0
        val decoder = Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPLACE).onUnmappableCharacter(CodingErrorAction.REPLACE)
        return decoder.decode(ByteBuffer.wrap(bytes, start, bytes.size - start)).toString()
    }

    sealed interface SaveRequest
    class SaveOk(val owner: String, val fields: JsObject) : SaveRequest
    class SaveRefused(val response: Response) : SaveRequest

    /** readSaveRequest: the login, then the body, then its accountId. */
    private fun readSaveRequest(request: Request, db: Database, login: LoginSettings, operation: String, invalid: String): SaveRequest {
        val owner = when (val account = Account.signedIn(request, Account.databaseSessions(db), login)) {
            is Account.Refusal -> return SaveRefused(account.response)
            is Account.User -> account.user.sub
        }
        val body = try {
            request.body()
        } catch (error: Exception) {
            Report.failure(request, error, operation)
            return SaveRefused(Answers.refuse(503, "workspaceSaveFailed"))
        }
        if (body.isEmpty()) return SaveRefused(Answers.refuse(400, "emptyRequest"))
        if (body.size > MAX_BODY_BYTES) return SaveRefused(Answers.refuse(413, "workspaceTooLarge"))
        val fields = try {
            JsonParser.parse(decodeBody(body))
        } catch (error: Exception) {
            return SaveRefused(Answers.refuse(400, invalid))
        }
        if (fields !is JsObject) return SaveRefused(Answers.refuse(400, invalid))
        if (fields["accountId"] != owner || !fields.containsKey("accountId")) return SaveRefused(Answers.refuse(401, "loginAccountChanged"))
        return SaveOk(owner, fields)
    }

    private fun readWorkspaceAnswer(request: Request, db: Database, login: LoginSettings): Response {
        val owner = when (val account = Account.signedIn(request, Account.databaseSessions(db), login)) {
            is Account.Refusal -> return account.response
            is Account.User -> account.user.sub
        }
        return try {
            Answers.reply(200, answer(db.withConnection { readSavedWorkspace(it, owner) }, owner))
        } catch (error: Exception) {
            Report.failure(request, error, "workspace.read")
            Answers.refuse(503, "databaseUnavailable")
        }
    }

    private fun saveWorkspaceAnswer(request: Request, db: Database, login: LoginSettings): Response {
        val save = when (val read = readSaveRequest(request, db, login, "workspace.write", "invalidWorkspace")) {
            is SaveRefused -> return read.response
            is SaveOk -> read
        }
        val read = readWorkspaceChange(save.fields)
        if (read is ChangeRefused) return Answers.refuse(400, read.key)
        read as ChangeOk
        return try {
            Answers.reply(200, answer(writeWorkspace(db, save.owner, read.revision, read.change), save.owner))
        } catch (error: Conflict) {
            Answers.refuse(409, "conflict")
        } catch (error: Exception) {
            if (error is AppError && error.key in listOf("listLimit", "followLimit", "invalidWorkspace")) Answers.refuse(400, error.key)
            else {
                Report.failure(request, error, "workspace.write")
                Answers.refuse(503, "workspaceSaveFailed")
            }
        }
    }

    /** /api/workspace: GET reads the owner's workspace, PUT saves a change of it by revision. */
    fun answerWorkspace(request: Request, open: () -> Database?, settings: () -> LoginSettings?): Response {
        val login = settings() ?: return Answers.refuse(503, "authUnavailable")
        if (request.method == "PUT" && (!RequestChecks.sameOrigin(request, login.origin) || !RequestChecks.jsonContent(request))) {
            return Answers.refuse(403, "forbiddenRequest")
        }
        val db = try {
            open() ?: throw IllegalStateException("DATABASE_URL is not set")
        } catch (error: Exception) {
            Report.failure(request, error, "workspace.open")
            return Answers.refuse(503, "databaseUnavailable")
        }
        if (request.method != "PUT") return readWorkspaceAnswer(request, db, login)
        return saveWorkspaceAnswer(request, db, login)
    }

    private val recordFields = setOf("id", "time", "date", "mistakes", "accuracy", "wordCount", "deviceId", "device", "mode", "missed")

    /** The list and record of a POST body (readRecordAdd), null when it is not exactly accountId, listId and record. */
    fun readRecordAdd(fields: JsObject): Pair<String, JsObject>? {
        val listId = prop(fields, "listId")
        val record = prop(fields, "record")
        if (fields.size != 3 || listId !is String || listId.isEmpty()) return null
        if (record !is JsObject) return null
        val input = JsObject(record)
        if (input.keys.any { it !in recordFields } || input["id"] !is String) return null
        for (name in listOf("wordCount", "device", "deviceId")) if (input.containsKey(name) && input[name] == null) input.remove(name)
        return listId to input
    }

    /** The list and record of a DELETE body (readRecordDelete). */
    fun readRecordDelete(fields: JsObject): Pair<String, String>? {
        val listId = prop(fields, "listId")
        val recordId = prop(fields, "recordId")
        if (fields.size != 3 || listId !is String || listId.isEmpty() || recordId !is String || recordId.isEmpty()) return null
        return listId to recordId
    }

    private fun recordRefusal(request: Request, error: Throwable, operation: String): Response {
        if (error is AppError && error.key == "recordListDeleted") return Answers.refuse(404, "recordListDeleted")
        if (error is AppError && error.key == "workspaceTooLarge") return Answers.refuse(413, "workspaceTooLarge")
        if (error is AppError) return Answers.refuse(400, error.key)
        Report.failure(request, error, operation)
        return Answers.refuse(503, "workspaceSaveFailed")
    }

    private class AddResult(val record: Any?, val added: Boolean)

    private fun changeRecordAnswer(request: Request, db: Database, login: LoginSettings): Response {
        val adding = request.method == "POST"
        val operation = if (adding) "record.add" else "record.delete"
        val save = when (val read = readSaveRequest(request, db, login, operation, "invalidRecord")) {
            is SaveRefused -> return read.response
            is SaveOk -> read
        }
        val outcome: Modified<AddResult?> = try {
            if (adding) {
                val (listId, input) = readRecordAdd(save.fields) ?: return Answers.refuse(400, "invalidRecord")
                modifyWorkspace(db, save.owner) { document ->
                    val added = Records.addRecord(document.lists, document.devices, listId, input)
                    Change(document.copy(lists = added.lists), added.added, AddResult(added.record, added.added))
                }
            } else {
                val (listId, recordId) = readRecordDelete(save.fields) ?: return Answers.refuse(400, "invalidRecord")
                modifyWorkspace(db, save.owner) { document ->
                    val lists = Records.deleteRecord(document.lists, listId, recordId)
                    Change(if (lists == null) document else document.copy(lists = lists), lists != null, null)
                }
            }
        } catch (error: Exception) {
            return recordRefusal(request, error, operation)
        }
        val saved = outcome.saved ?: return recordRefusal(request, IllegalStateException("workspace missing after a record was changed"), operation)
        val answer = answer(saved, save.owner)
        if (!adding) return Answers.reply(200, answer)
        val result = outcome.result!!
        return Answers.reply(if (result.added) 201 else 200, answer.apply { put("record", result.record) })
    }

    /** /api/workspace/records: POST adds a record, DELETE deletes one. */
    fun answerRecords(request: Request, open: () -> Database?, settings: () -> LoginSettings?): Response {
        val login = settings() ?: return Answers.refuse(503, "authUnavailable")
        if (!RequestChecks.sameOrigin(request, login.origin) || !RequestChecks.jsonContent(request)) return Answers.refuse(403, "forbiddenRequest")
        val db = try {
            open() ?: throw IllegalStateException("DATABASE_URL is not set")
        } catch (error: Exception) {
            Report.failure(request, error, "workspace.open")
            return Answers.refuse(503, "databaseUnavailable")
        }
        return changeRecordAnswer(request, db, login)
    }
}
