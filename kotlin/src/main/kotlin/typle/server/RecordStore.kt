package typle.server

import typle.lib.JsArray
import typle.lib.JsDate
import typle.lib.JsObject
import typle.lib.JsonWriter
import typle.lib.goFloat
import typle.lib.prop
import java.security.MessageDigest
import java.sql.Connection

// src/server/records.ts: practice records kept in typle_records, one row each, and put back in their lists on reads.

/** A record as typle_records keeps it. at is milliseconds since the epoch. */
data class RecordRow(val listId: String, val recordId: String, val position: Int, val at: Double, val data: String)

object RecordStore {
    /** The id a record without one (or with an earlier record's id) is given: the same record always gets the same id. */
    fun legacyId(listId: String, position: Int, date: String, time: Double): String {
        val digest = MessageDigest.getInstance("SHA-256").digest("$listId\u0000$position\u0000$date\u0000${goFloat(time)}".toByteArray(Charsets.UTF_8))
        return "legacy-" + digest.joinToString("") { "%02x".format(it) }.substring(0, 20)
    }

    /** The records of the lists as rows, every record given an id that keeps it apart from the rest of its list, in place. */
    fun recordRows(lists: List<JsObject>): List<RecordRow> {
        val rows = ArrayList<RecordRow>()
        for (list in lists) {
            val seen = HashSet<String>()
            val listId = list["id"] as String
            (list["records"] as JsArray).forEachIndexed { position, item ->
                val record = item as JsObject
                val id = record["id"]
                if (id !is String || id.isEmpty() || id in seen) {
                    record["id"] = legacyId(listId, position, record["date"] as String, record["time"] as Double)
                }
                val recordId = record["id"] as String
                seen.add(recordId)
                rows.add(RecordRow(listId, recordId, position, JsDate.parse(record["date"] as String), JsonWriter.stringify(record)))
            }
        }
        return rows
    }

    /** The lists with the stored records put back in the lists that hold them (withRecords). */
    fun withRecords(lists: List<Any?>, stored: Map<String, List<Any?>>): JsArray {
        val result = JsArray()
        for (item in lists) {
            if (item == null) throw IllegalStateException("Cannot read properties of null (reading 'id')")
            val id = prop(item, "id")
            val moved = if (id is String) stored[id] ?: emptyList() else emptyList()
            if (moved.isEmpty()) { result.add(item); continue }
            val movedIds = moved.map(::recordIdOf).toSet()
            val own = prop(item, "records") as? JsArray ?: JsArray()
            val kept = own.filter { val rid = recordIdOf(it); rid == "" || rid !in movedIds }
            val list = item as? JsObject ?: JsObject()
            result.add(JsObject(list).apply { put("records", JsArray(kept + moved)) })
        }
        return result
    }

    private fun recordIdOf(record: Any?) = (prop(record, "id") as? String) ?: ""

    /** A timestamptz PostgreSQL reads, of a time in milliseconds (node-postgres writes a Date so, in UTC here). */
    private fun pgTimestamp(time: Double): String {
        val iso = JsDate.toIsoString(time)
        val year = JsDate.utcYear(time).toLong()
        if (year in 1..9999) return iso
        val rest = iso.substring(iso.indexOf('-', 1))
        return if (year > 9999) "$year$rest" else (1 - year).toString().padStart(4, '0') + rest.removeSuffix("Z") + "Z BC"
    }

    /**
     * Makes the owner's rows of typle_records exactly rows (syncRecords): the rows of records no longer saved are deleted,
     * the others written where they changed, and the number of rows checked afterwards.
     */
    fun syncRecords(connection: Connection, owner: String, rows: List<RecordRow>) {
        val lists = connection.createArrayOf("text", rows.map { it.listId }.toTypedArray())
        val ids = connection.createArrayOf("text", rows.map { it.recordId }.toTypedArray())
        connection.execute(
            """DELETE FROM typle_records r WHERE r.owner_id = ? AND NOT EXISTS (SELECT 1 FROM
    unnest(?::text[], ?::text[]) AS k(list_id, record_id) WHERE k.list_id = r.list_id AND k.record_id = r.record_id)""",
            owner, lists, ids,
        )
        if (rows.isNotEmpty()) {
            val positions = connection.createArrayOf("int4", rows.map { it.position }.toTypedArray())
            val ats = connection.createArrayOf("text", rows.map { pgTimestamp(it.at) }.toTypedArray())
            val data = connection.createArrayOf("text", rows.map { it.data }.toTypedArray())
            connection.execute(
                """INSERT INTO typle_records (owner_id, list_id, record_id, position, at, data)
      SELECT ?, k.list_id, k.record_id, k.position, k.at, k.data
      FROM unnest(?::text[], ?::text[], ?::int[], ?::text[]::timestamptz[], ?::text[]::jsonb[]) AS k(list_id, record_id, position, at, data)
      ON CONFLICT (owner_id, list_id, record_id) DO UPDATE SET position = EXCLUDED.position, at = EXCLUDED.at, data = EXCLUDED.data
      WHERE (typle_records.position, typle_records.at, typle_records.data) IS DISTINCT FROM (EXCLUDED.position, EXCLUDED.at, EXCLUDED.data)""",
                owner, lists, ids, positions, ats, data,
            )
        }
        val counted = connection.query("SELECT count(*)::int AS n FROM typle_records WHERE owner_id = ?", owner) { it.next(); it.getInt(1) }
        if (counted != rows.size) throw IllegalStateException("records of $owner: $counted rows saved for ${rows.size} records")
    }
}
