package typle.server

import java.io.ByteArrayOutputStream
import java.net.URI
import java.security.MessageDigest
import java.util.Base64
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

// src/server/settings.ts, src/server/authUser.ts and src/server/betterAuth.ts: the login settings, and the login cookie
// Better Auth sets, the session token signed with BETTER_AUTH_SECRET as better-call's signCookieValue signs it. The
// cookie only names a session; who is signed in, and until when, is the session's row in typle_auth_sessions
// (Account.databaseSessions). Only reading is served here: typle-r signs the users in, and sign is kept for the tests.

/**
 * The login settings these endpoints need: the app's origin (AUTH_URL) and whether it is https, BETTER_AUTH_SECRET
 * (empty when it is not set) and whether the shared demo account's login is on (DEMO_LOGIN is exactly 1).
 */
class LoginSettings(val origin: String, val secret: String, val demoLogin: Boolean, val secure: Boolean) {
    companion object {
        /**
         * readLoginSettings: null when AUTH_URL is not an http(s) URL with a host, which the routes answer with 503
         * authUnavailable. BETTER_AUTH_SECRET is read trimmed, as betterAuthSettings of src/server/authUser.ts reads it;
         * without it the routes that need the login answer 503 databaseUnavailable (Account.signedIn).
         */
        fun read(env: Map<String, String>): LoginSettings? {
            val base = WhatwgUrl.origin(env["AUTH_URL"] ?: "") ?: return null
            return LoginSettings(base.first, (env["BETTER_AUTH_SECRET"] ?: "").trim(), env["DEMO_LOGIN"] == "1", base.second)
        }
    }

    /** The session cookie's name (Sessions.cookieName). */
    val cookieName get() = Sessions.cookieName(secure)
}

/** The origin of an http or https URL as new URL(text).origin gives it, and whether it is https; null otherwise. */
object WhatwgUrl {
    fun origin(text: String): Pair<String, Boolean>? {
        val trimmed = text.trim { it <= ' ' }
        val uri = try { URI(trimmed) } catch (_: Exception) { return null }
        val scheme = uri.scheme?.lowercase() ?: return null
        if (scheme != "http" && scheme != "https") return null
        val host = uri.host?.lowercase() ?: return null
        if (host.isEmpty()) return null
        val defaultPort = if (scheme == "https") 443 else 80
        val port = if (uri.port == -1 || uri.port == defaultPort) "" else ":${uri.port}"
        return "$scheme://$host$port" to (scheme == "https")
    }
}

/** A signed-in user: sub is the account ID (typle_auth_users.id, the owner_id of the account's data). */
data class Session(val sub: String, val name: String, val image: String)

object Sessions {
    /**
     * The length of an HMAC-SHA256 in standard base64 with its padding, the only length better-call's getSignedCookie
     * accepts.
     */
    private const val SIGNATURE_LENGTH = 44

    /**
     * The name of Better Auth's session cookie: better-auth.session_token, with the __Secure- prefix when AUTH_URL is
     * HTTPS (createCookieGetter in better-auth's cookies).
     */
    fun cookieName(secure: Boolean) = if (secure) "__Secure-better-auth.session_token" else "better-auth.session_token"

    /**
     * The session token of a session cookie's value as the Cookie header carries it, as better-call's getSignedCookie
     * reads it: the value is URL-decoded (kept as it is when it cannot be), then split at its last "." into the token
     * and its signature, which must be the HMAC-SHA256 of the token under secret in standard base64. null for any other
     * value, and for an empty secret.
     */
    fun token(value: String, secret: String): String? {
        if (secret.isEmpty()) return null
        val bytes = percentDecode(value) ?: value.toByteArray(Charsets.UTF_8)
        val at = bytes.lastIndexOf('.'.code.toByte())
        if (at < 1) return null
        val token = bytes.copyOfRange(0, at)
        val signature = String(bytes.copyOfRange(at + 1, bytes.size), Charsets.ISO_8859_1)
        if (signature.length != SIGNATURE_LENGTH || !signature.endsWith("=")) return null
        val sent = try { Base64.getDecoder().decode(signature) } catch (_: IllegalArgumentException) { return null }
        if (!MessageDigest.isEqual(sent, mac(token, secret))) return null
        return String(token, Charsets.UTF_8)
    }

    /**
     * The cookie value Better Auth sets for token (signCookieValue): the token, ".", its signature, URL-encoded as
     * encodeURIComponent encodes it.
     */
    fun sign(token: String, secret: String): String {
        val bytes = token.toByteArray(Charsets.UTF_8)
        return encodeURIComponent(bytes + ".".toByteArray() + Base64.getEncoder().encode(mac(bytes, secret)))
    }

    private fun mac(token: ByteArray, secret: String): ByteArray =
        Mac.getInstance("HmacSHA256").apply { init(SecretKeySpec(secret.toByteArray(Charsets.UTF_8), "HmacSHA256")) }.doFinal(token)

    /** The bytes of text with its %XX escapes decoded (a "+" stays a "+"), or null when an escape is not two hex digits. */
    private fun percentDecode(text: String): ByteArray? {
        val bytes = text.toByteArray(Charsets.UTF_8)
        val out = ByteArrayOutputStream(bytes.size)
        var i = 0
        while (i < bytes.size) {
            val b = bytes[i]
            if (b != '%'.code.toByte()) {
                out.write(b.toInt())
                i++
                continue
            }
            if (i + 2 >= bytes.size) return null
            val high = Character.digit(bytes[i + 1].toInt().toChar(), 16)
            val low = Character.digit(bytes[i + 2].toInt().toChar(), 16)
            if (high < 0 || low < 0 || bytes[i + 1] < 0 || bytes[i + 2] < 0) return null
            out.write(high * 16 + low)
            i += 3
        }
        return out.toByteArray()
    }

    /** What JavaScript's encodeURIComponent escapes: every byte but A-Z a-z 0-9 - _ . ! ~ * ' ( ). */
    private fun encodeURIComponent(bytes: ByteArray): String {
        val hex = "0123456789ABCDEF"
        val out = StringBuilder()
        for (b in bytes) {
            val c = b.toInt() and 0xff
            if (c in 'A'.code..'Z'.code || c in 'a'.code..'z'.code || c in '0'.code..'9'.code || "-_.!~*'()".indexOf(c.toChar()) >= 0) {
                out.append(c.toChar())
            } else {
                out.append('%').append(hex[c shr 4]).append(hex[c and 15])
            }
        }
        return out.toString()
    }
}
