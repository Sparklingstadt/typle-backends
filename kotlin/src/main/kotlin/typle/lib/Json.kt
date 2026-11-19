package typle.lib

import java.math.BigDecimal

// JSON as JavaScript holds it, so that what the Next.js backend parses, changes and writes back is parsed, changed and
// written here byte for byte the same way:
// - an object is a JsObject (insertion order kept, as a JS object keeps it for its non-index keys),
// - an array a JsArray, a string a String (UTF-16, lone surrogates allowed), a number a Double, true/false a Boolean,
// - null is Kotlin null, and a missing property reads as Undefined.
// stringify writes as JSON.stringify does: index-like keys first, numbers as Number.prototype.toString writes them.

class JsObject() : LinkedHashMap<String, Any?>() {
    constructor(from: Map<String, Any?>) : this() {
        putAll(from)
    }
}

class JsArray() : ArrayList<Any?>() {
    constructor(from: Collection<Any?>) : this() {
        addAll(from)
    }

    @Suppress("UNUSED_PARAMETER")
    constructor(capacity: Int) : this()
}

/** The value of a property that is not there (JavaScript's undefined). Never stored in an object or array. */
object Undefined

/** A body that is not JSON, as JSON.parse refuses it. */
class JsonSyntaxError(message: String) : Exception(message)

fun jsObject(vararg pairs: Pair<String, Any?>): JsObject = JsObject().apply { for ((k, v) in pairs) put(k, v) }
fun jsArray(items: Iterable<Any?>): JsArray = JsArray().apply { addAll(items) }

/** value.key as JavaScript reads it: a property of an object, and undefined on anything else. */
fun prop(value: Any?, key: String): Any? {
    if (value is JsObject) return if (value.containsKey(key)) value[key] else Undefined
    return Undefined
}

/** A shallow copy ({ ...value }). */
fun JsObject.copy(): JsObject = JsObject(this)

/** { ...this } without the keys. */
fun JsObject.without(vararg keys: String): JsObject = copy().apply { for (k in keys) remove(k) }

/** { ...this, key: value }: an existing key keeps its place, a new one goes last. */
fun JsObject.with(key: String, value: Any?): JsObject = copy().apply { put(key, value) }

/** A deep copy, as JSON.parse(JSON.stringify(value)) makes one (-0 becomes 0, other values stay). */
fun deepCopy(value: Any?): Any? = when (value) {
    is JsObject -> JsObject().also { out -> for ((k, v) in value) out[k] = deepCopy(v) }
    is JsArray -> JsArray(value.size).also { out -> for (v in value) out.add(deepCopy(v)) }
    is Double -> if (value == 0.0) 0.0 else if (value.isFinite()) value else null
    else -> value
}

fun isString(value: Any?) = value is String
fun isNumber(value: Any?) = value is Double
fun isFiniteNumber(value: Any?) = value is Double && value.isFinite()

/** Number.isInteger. */
fun isInteger(value: Any?) = value is Double && value.isFinite() && Math.floor(value) == value

/** Number.isSafeInteger. */
fun isSafeInteger(value: Any?) = isInteger(value) && Math.abs(value as Double) <= 9007199254740991.0

/** typeof value === 'object' && value !== null, for objects and arrays alike. */
fun isObjectLike(value: Any?) = value is JsObject || value is JsArray

/** A JSON text read as JSON.parse reads it. */
object JsonParser {
    fun parse(text: String): Any? {
        val reader = Reader(text)
        reader.skipSpace()
        val value = reader.value(0)
        reader.skipSpace()
        if (reader.at < text.length) throw JsonSyntaxError("Unexpected token at ${reader.at}")
        return value
    }

    private class Reader(val text: String) {
        var at = 0

        fun skipSpace() {
            while (at < text.length) {
                val c = text[at]
                if (c == ' ' || c == '\t' || c == '\n' || c == '\r') at++ else break
            }
        }

        fun fail(): Nothing = throw JsonSyntaxError(if (at >= text.length) "Unexpected end of JSON input" else "Unexpected token at $at")

        fun value(depth: Int): Any? {
            if (depth > 4000) throw JsonSyntaxError("too deep")
            if (at >= text.length) fail()
            return when (val c = text[at]) {
                '{' -> objectValue(depth)
                '[' -> arrayValue(depth)
                '"' -> string()
                't' -> literal("true", true)
                'f' -> literal("false", false)
                'n' -> literal("null", null)
                else -> if (c == '-' || c in '0'..'9') number() else fail()
            }
        }

        fun literal(word: String, value: Any?): Any? {
            if (!text.startsWith(word, at)) fail()
            at += word.length
            return value
        }

        fun objectValue(depth: Int): JsObject {
            at++
            val result = JsObject()
            skipSpace()
            if (at < text.length && text[at] == '}') { at++; return result }
            while (true) {
                skipSpace()
                if (at >= text.length || text[at] != '"') fail()
                val key = string()
                skipSpace()
                if (at >= text.length || text[at] != ':') fail()
                at++
                skipSpace()
                result[key] = value(depth + 1)
                skipSpace()
                if (at >= text.length) fail()
                when (text[at]) {
                    ',' -> at++
                    '}' -> { at++; return result }
                    else -> fail()
                }
            }
        }

        fun arrayValue(depth: Int): JsArray {
            at++
            val result = JsArray()
            skipSpace()
            if (at < text.length && text[at] == ']') { at++; return result }
            while (true) {
                skipSpace()
                result.add(value(depth + 1))
                skipSpace()
                if (at >= text.length) fail()
                when (text[at]) {
                    ',' -> at++
                    ']' -> { at++; return result }
                    else -> fail()
                }
            }
        }

        fun string(): String {
            at++
            val out = StringBuilder()
            while (true) {
                if (at >= text.length) fail()
                val c = text[at++]
                when {
                    c == '"' -> return out.toString()
                    c == '\\' -> {
                        if (at >= text.length) fail()
                        when (text[at++]) {
                            '"' -> out.append('"')
                            '\\' -> out.append('\\')
                            '/' -> out.append('/')
                            'b' -> out.append('\b')
                            'f' -> out.append('\u000c')
                            'n' -> out.append('\n')
                            'r' -> out.append('\r')
                            't' -> out.append('\t')
                            'u' -> {
                                if (at + 4 > text.length) { at = text.length; fail() }
                                var code = 0
                                for (i in 0 until 4) {
                                    val digit = Character.digit(text[at + i], 16)
                                    if (digit < 0 || text[at + i].code > 127) { at += i; fail() }
                                    code = code * 16 + digit
                                }
                                at += 4
                                out.append(code.toChar())
                            }
                            else -> { at--; fail() }
                        }
                    }
                    c.code < 0x20 -> { at--; fail() }
                    else -> out.append(c)
                }
            }
        }

        fun number(): Double {
            val start = at
            if (text[at] == '-') at++
            if (at >= text.length) fail()
            if (text[at] == '0') at++
            else if (text[at] in '1'..'9') while (at < text.length && text[at] in '0'..'9') at++
            else fail()
            if (at < text.length && text[at] == '.') {
                at++
                if (at >= text.length || text[at] !in '0'..'9') fail()
                while (at < text.length && text[at] in '0'..'9') at++
            }
            if (at < text.length && (text[at] == 'e' || text[at] == 'E')) {
                at++
                if (at < text.length && (text[at] == '+' || text[at] == '-')) at++
                if (at >= text.length || text[at] !in '0'..'9') fail()
                while (at < text.length && text[at] in '0'..'9') at++
            }
            return text.substring(start, at).toDouble()
        }
    }
}

/** JSON.stringify of a value made of the types above. Undefined values of an object are left out, as JS does. */
object JsonWriter {
    fun stringify(value: Any?): String = StringBuilder().also { write(it, value) }.toString()

    private fun write(out: StringBuilder, value: Any?) {
        when (value) {
            null -> out.append("null")
            is Boolean -> out.append(if (value) "true" else "false")
            is Double -> out.append(if (value.isFinite()) jsNumber(value) else "null")
            is Int -> out.append(value)
            is Long -> out.append(value)
            is String -> quote(out, value)
            is JsArray -> {
                out.append('[')
                value.forEachIndexed { i, item ->
                    if (i > 0) out.append(',')
                    if (item === Undefined) out.append("null") else write(out, item)
                }
                out.append(']')
            }
            is JsObject -> {
                out.append('{')
                var first = true
                for (key in jsKeys(value)) {
                    val item = value[key]
                    if (item === Undefined) continue
                    if (!first) out.append(',')
                    first = false
                    quote(out, key)
                    out.append(':')
                    write(out, item)
                }
                out.append('}')
            }
            is Map<*, *> -> {
                @Suppress("UNCHECKED_CAST")
                write(out, JsObject(value as Map<String, Any?>))
            }
            is List<*> -> write(out, JsArray(value as Collection<Any?>))
            else -> throw IllegalArgumentException("not JSON: ${value::class}")
        }
    }

    private fun quote(out: StringBuilder, text: String) {
        out.append('"')
        var i = 0
        while (i < text.length) {
            val c = text[i]
            when {
                c == '"' -> out.append("\\\"")
                c == '\\' -> out.append("\\\\")
                c == '\b' -> out.append("\\b")
                c == '\u000c' -> out.append("\\f")
                c == '\n' -> out.append("\\n")
                c == '\r' -> out.append("\\r")
                c == '\t' -> out.append("\\t")
                c.code < 0x20 -> out.append("\\u").append(String.format("%04x", c.code))
                Character.isHighSurrogate(c) && i + 1 < text.length && Character.isLowSurrogate(text[i + 1]) -> {
                    out.append(c).append(text[i + 1])
                    i++
                }
                Character.isSurrogate(c) -> out.append("\\u").append(String.format("%04x", c.code))
                else -> out.append(c)
            }
            i++
        }
        out.append('"')
    }
}

/** Whether key is an array index, which a JS object lists before its other keys, in ascending order. */
private fun isIndexKey(key: String): Boolean {
    if (key.isEmpty() || key.length > 10) return false
    if (key == "0") return true
    if (key[0] !in '1'..'9' || key.any { it !in '0'..'9' }) return false
    return key.toLong() <= 4294967294L
}

/** Object.keys(value): index keys in ascending order, then the others in the order they were added. */
fun jsKeys(value: Map<String, Any?>): List<String> {
    val keys = value.keys
    if (keys.none(::isIndexKey)) return keys.toList()
    return keys.filter(::isIndexKey).sortedBy { it.toLong() } + keys.filterNot(::isIndexKey)
}

/** The shortest digits of a finite, non-zero magnitude and n, with magnitude = 0.digits × 10^n (ECMAScript's k, n). */
private fun shortestDigits(magnitude: Double): Pair<String, Int> {
    // Since JDK 19, Double.toString gives the shortest decimal that reads back as the same double, the closest such.
    // It gives two digits where one would do (4.9E-324), which ECMAScript writes with one (5e-324): a digit that reads back
    // as the same double is taken, the closer one when both do.
    val decimal = BigDecimal(java.lang.Double.toString(magnitude)).stripTrailingZeros()
    val digits = decimal.unscaledValue().toString()
    val n = digits.length - decimal.scale()
    if (digits.length == 2) {
        val exact = BigDecimal(magnitude)
        val low = digits[0].digitToInt()
        val candidates = listOf(low to n, (low + 1) to n).map { (d, at) -> if (d == 10) 1 to at + 1 else d to at }
            .filter { (d, at) -> "${d}e${at - 1}".toDouble() == magnitude }
        val best = candidates.minByOrNull { (d, at) -> BigDecimal("${d}e${at - 1}").subtract(exact).abs() }
        if (best != null) return best.first.toString() to best.second
    }
    return digits to n
}

/** Number.prototype.toString of a finite number (JSON.stringify writes numbers so; -0 is "0"). */
fun jsNumber(value: Double): String {
    if (value == 0.0) return "0"
    if (value < 0) return "-" + jsNumber(-value)
    val (digits, n) = shortestDigits(value)
    val k = digits.length
    return when {
        n in k..21 -> digits + "0".repeat(n - k)
        n in 1..21 -> digits.substring(0, n) + "." + digits.substring(n)
        n in -5..0 -> "0." + "0".repeat(-n) + digits
        else -> {
            val e = n - 1
            val exponent = (if (e < 0) "-" else "+") + Math.abs(e)
            (if (k == 1) digits else digits[0] + "." + digits.substring(1)) + "e" + exponent
        }
    }
}

/**
 * value as Go's strconv.FormatFloat(value, 'g', -1, 64) writes it: the shortest digits, in exponent form (two digits at
 * least, with a sign) when the exponent is below -4 or 6 and above, and plain otherwise (src/server/records.ts goFloat).
 */
fun goFloat(value: Double): String {
    if (value == 0.0) return if (1.0 / value < 0) "-0" else "0"
    if (value < 0) return "-" + goFloat(-value)
    val (digits, n) = shortestDigits(value)
    val exponent = n - 1
    if (exponent < -4 || exponent >= 6) {
        val mantissa = if (digits.length == 1) digits else digits[0] + "." + digits.substring(1)
        return mantissa + "e" + (if (exponent < 0) "-" else "+") + Math.abs(exponent).toString().padStart(2, '0')
    }
    return jsNumber(value)
}
