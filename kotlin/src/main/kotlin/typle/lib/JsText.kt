package typle.lib

import java.time.LocalDate
import java.util.Locale

// Strings and dates as JavaScript treats them: trim() and \s use ECMAScript's white space, toLocaleLowerCase() the
// default locale's (Node's is not Turkish or Lithuanian, so the root rules), Date.parse() V8's formats.

/** Whether c is white space or a line terminator for String.prototype.trim and the \s of a regular expression. */
fun isJsSpace(c: Char): Boolean = when (c) {
    '\u0009', '\u000a', '\u000b', '\u000c', '\u000d', ' ', ' ', ' ', ' ', ' ', ' ',
    ' ', '　', '﻿' -> true
    else -> c in ' '..' '
}

fun String.jsTrim(): String = trim(::isJsSpace)
fun String.jsTrimEnd(): String = trimEnd(::isJsSpace)

/** String.prototype.toLocaleLowerCase in Node's default locale. */
fun String.jsLower(): String = lowercase(Locale.ROOT)

/** String.prototype.slice(0, end) with JavaScript's negative end. */
fun String.jsSliceTo(end: Int): String {
    val stop = if (end < 0) maxOf(length + end, 0) else minOf(end, length)
    return substring(0, stop)
}

/** Dates as V8's Date.parse reads them and Date.prototype.toISOString writes them. Times are milliseconds (Double). */
object JsDate {
    private const val MAX_TIME = 8.64e15
    private const val DAY_MS = 86_400_000.0

    /** ECMAScript's MakeDay (with month overflow) and MakeDate, in UTC. */
    private fun makeTime(year: Long, month: Long, day: Long, hour: Long, minute: Long, second: Long, ms: Long): Double {
        val y = year + Math.floorDiv(month, 12L)
        val m = Math.floorMod(month, 12L)
        if (Math.abs(y) > 400_000) return Double.NaN
        val days = LocalDate.of(y.toInt(), (m + 1).toInt(), 1).toEpochDay() + day - 1
        return days * DAY_MS + ((hour * 60 + minute) * 60 + second) * 1000.0 + ms
    }

    private fun clip(time: Double): Double = if (time.isNaN() || Math.abs(time) > MAX_TIME) Double.NaN else time

    // ECMAScript's date time string format, as V8 reads it first: a date alone is UTC, a date and time without an offset
    // is local time (UTC on Vercel and Cloud Run, where the servers run).
    private val iso = Regex(
        "^([+-]\\d{6}|\\d{4})(?:-(\\d{2})(?:-(\\d{2}))?)?" +
            "(?:T(\\d{2}):(\\d{2})(?::(\\d{2})(?:\\.(\\d+))?)?)?(Z|[+-]\\d{2}:\\d{2})?$",
    )

    /** Date.parse(text): milliseconds since the epoch, or NaN. */
    fun parse(text: String): Double {
        val match = iso.find(text)
        if (match != null) return parseIso(match)
        return parseLegacy(text)
    }

    private fun parseIso(match: MatchResult): Double {
        val g = match.groupValues
        if (g[1] == "-000000") return Double.NaN
        val year = g[1].toLong()
        val month = if (g[2].isEmpty()) 1L else g[2].toLong()
        val day = if (g[3].isEmpty()) 1L else g[3].toLong()
        val hasTime = g[4].isNotEmpty()
        val hour = if (hasTime) g[4].toLong() else 0L
        val minute = if (hasTime) g[5].toLong() else 0L
        val second = if (g[6].isEmpty()) 0L else g[6].toLong()
        val ms = if (g[7].isEmpty()) 0L else (g[7] + "00").substring(0, 3).toLong()
        if (month !in 1..12 || day !in 1..31 || minute > 59 || second > 59) return Double.NaN
        if (hour > 24 || (hour == 24L && (minute != 0L || second != 0L || ms != 0L))) return Double.NaN
        if (!hasTime && g[8].isNotEmpty()) return Double.NaN
        var time = makeTime(year, month - 1, day, hour, minute, second, ms)
        val zone = g[8]
        if (zone.isNotEmpty() && zone != "Z") {
            val sign = if (zone[0] == '-') -1 else 1
            val offHour = zone.substring(1, 3).toLong()
            val offMinute = zone.substring(4, 6).toLong()
            if (offHour > 23 || offMinute > 59) return Double.NaN
            time -= sign * (offHour * 60 + offMinute) * 60_000.0
        }
        return clip(time)
    }

    private val months = listOf("jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec")

    // A part of V8's legacy parser, for the forms other than ISO that a browser or app could have written: "2026-09-07
    // 10:00:00", "2026/09/07 10:00", "Mon, 07 Sep 2026 10:00:00 GMT", "Mon Sep 07 2026 10:00:00 GMT+0900 (JST)",
    // "Sep 7, 2026". Without a zone it is local time (UTC here).
    private val numeric = Regex(
        "^\\s*(\\d{1,6})[-/](\\d{1,2})[-/](\\d{1,2})(?:[T ]+(\\d{1,2}):(\\d{2})(?::(\\d{2})(?:\\.(\\d+))?)?)?\\s*" +
            "(Z|UTC|GMT|(?:(?:UTC|GMT)?[+-]\\d{2}:?\\d{2}))?\\s*$",
        RegexOption.IGNORE_CASE,
    )
    private val worded = Regex(
        "^\\s*(?:[A-Za-z]+,?\\s+)?(?:(\\d{1,2})\\s+([A-Za-z]+)|([A-Za-z]+)\\s+(\\d{1,2}),?)\\s+(\\d{1,6})" +
            "(?:\\s+(\\d{1,2}):(\\d{2})(?::(\\d{2}))?)?\\s*((?:UTC|GMT|Z)?(?:[+-]\\d{2}:?\\d{2})?)\\s*(?:\\([^)]*\\))?\\s*$",
        RegexOption.IGNORE_CASE,
    )

    private fun parseLegacy(text: String): Double {
        numeric.find(text)?.let { m ->
            val g = m.groupValues
            val ms = if (g[7].isEmpty()) 0L else (g[7] + "00").substring(0, 3).toLong()
            return legacyTime(g[1].toLong(), g[2].toLong(), g[3].toLong(), g[4], g[5], g[6], ms, g[8])
        }
        worded.find(text)?.let { m ->
            val g = m.groupValues
            val (dayText, monthText) = if (g[1].isNotEmpty()) g[1] to g[2] else g[4] to g[3]
            val month = months.indexOf(monthText.lowercase(Locale.ROOT).take(3))
            if (month < 0 || monthText.length < 3) return Double.NaN
            return legacyTime(g[5].toLong(), month + 1L, dayText.toLong(), g[6], g[7], g[8], 0, g[9])
        }
        return Double.NaN
    }

    private fun legacyTime(year: Long, month: Long, day: Long, hour: String, minute: String, second: String, ms: Long, zone: String): Double {
        if (month !in 1..12 || day !in 1..31) return Double.NaN
        val h = if (hour.isEmpty()) 0L else hour.toLong()
        val mi = if (minute.isEmpty()) 0L else minute.toLong()
        val s = if (second.isEmpty()) 0L else second.toLong()
        if (h > 24 || mi > 59 || s > 59) return Double.NaN
        var time = makeTime(year, month - 1, day, h, mi, s, ms)
        val offset = Regex("([+-])(\\d{2}):?(\\d{2})$").find(zone)
        if (offset != null) {
            val sign = if (offset.groupValues[1] == "-") -1 else 1
            time -= sign * (offset.groupValues[2].toLong() * 60 + offset.groupValues[3].toLong()) * 60_000.0
        }
        return clip(time)
    }

    /** The UTC year of a time (getUTCFullYear), NaN for NaN. */
    fun utcYear(time: Double): Double {
        if (time.isNaN()) return Double.NaN
        val days = Math.floorDiv(time.toLong(), DAY_MS.toLong())
        return LocalDate.ofEpochDay(days).year.toDouble()
    }

    /** Date.prototype.toISOString of a valid time. */
    fun toIsoString(time: Double): String {
        require(!time.isNaN()) { "Invalid time value" }
        val t = time.toLong()
        val days = Math.floorDiv(t, DAY_MS.toLong())
        val inDay = Math.floorMod(t, DAY_MS.toLong())
        val date = LocalDate.ofEpochDay(days)
        val year = date.year
        val yearText = if (year in 0..9999) year.toString().padStart(4, '0')
        else (if (year < 0) "-" else "+") + Math.abs(year).toString().padStart(6, '0')
        val hour = inDay / 3_600_000
        val minute = inDay / 60_000 % 60
        val second = inDay / 1000 % 60
        val milli = inDay % 1000
        return "%s-%02d-%02dT%02d:%02d:%02d.%03dZ".format(yearText, date.monthValue, date.dayOfMonth, hour, minute, second, milli)
    }
}
