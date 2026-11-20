package typle.server

import typle.lib.JsObject
import typle.lib.JsonParser
import typle.lib.isInteger
import java.net.URI
import java.security.MessageDigest
import java.util.Base64
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

// src/server/settings.ts and src/server/session.ts: the keys derived from AUTH_SECRET, and the login cookie and the
// apps' bearer token, base64url(JSON) + "." + base64url(HMAC-SHA256 of that text), as the Go backend sealed them.

/** The login settings these endpoints need: the app's origin and the keys of the cookie and of the apps' token. */
class LoginSettings(val origin: String, val sessionKey: ByteArray, val appKey: ByteArray, val secure: Boolean) {
    companion object {
        private const val SESSION_INFO = "typle session v1"
        private const val APP_INFO = "typle app session v1"

        /** HKDF-SHA256 of secret with no salt, 32 bytes, as node:crypto's hkdfSync derives them. */
        fun deriveKey(secret: String, info: String): ByteArray {
            fun hmac(key: ByteArray, data: ByteArray): ByteArray =
                Mac.getInstance("HmacSHA256").apply { init(SecretKeySpec(key, "HmacSHA256")) }.doFinal(data)
            // An empty salt is HashLen zero bytes (RFC 5869).
            val prk = hmac(ByteArray(32), secret.toByteArray(Charsets.UTF_8))
            return hmac(prk, info.toByteArray(Charsets.UTF_8) + byteArrayOf(1)).copyOf(32)
        }

        /** readLoginSettings: null when AUTH_SECRET is empty or AUTH_URL is not an http(s) URL with a host. */
        fun read(env: Map<String, String>): LoginSettings? {
            val secret = env["AUTH_SECRET"] ?: ""
            val base = WhatwgUrl.origin(env["AUTH_URL"] ?: "") ?: return null
            if (secret.isEmpty()) return null
            return LoginSettings(base.first, deriveKey(secret, SESSION_INFO), deriveKey(secret, APP_INFO), base.second)
        }
    }

    /** The login cookie's name: __Host- over HTTPS. */
    val cookieName get() = if (secure) "__Host-typle-session" else "typle-session"
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

/** A signed-in user; times are Unix seconds. */
data class Session(val sub: String, val name: String, val image: String, val issuedAt: Long, val expires: Long)

object Sessions {
    private val accountSub = Regex("^(github:\\d+|password:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|ffpf:[a-z0-9]{1,64})$")
    private val base64url = Regex("^[A-Za-z0-9_-]*$")

    /** The bytes of strictly written base64url text (no padding, unused bits zero), or null. */
    fun strictBase64url(text: String): ByteArray? {
        if (!base64url.matches(text) || text.length % 4 == 1) return null
        val bytes = try { Base64.getUrlDecoder().decode(text) } catch (_: IllegalArgumentException) { return null }
        return if (Base64.getUrlEncoder().withoutPadding().encodeToString(bytes) == text) bytes else null
    }

    private fun openClaims(key: ByteArray, value: String): JsObject? {
        val parts = value.split('.')
        if (parts.size != 2) return null
        val (payload, signature) = parts
        val got = strictBase64url(signature)
        val want = Mac.getInstance("HmacSHA256").apply { init(SecretKeySpec(key, "HmacSHA256")) }
            .doFinal(payload.toByteArray(Charsets.UTF_8))
        if (got == null || got.size != want.size || !MessageDigest.isEqual(got, want)) return null
        val data = strictBase64url(payload) ?: return null
        // Buffer.toString('utf8') replaces malformed bytes with U+FFFD, as a String made with UTF_8 does.
        val claims = try { JsonParser.parse(String(data, Charsets.UTF_8)) } catch (_: Exception) { return null }
        return claims as? JsObject
    }

    /** The session in value if it was sealed with key, has not expired at now, and is for a GitHub or FFPF ID account. */
    fun open(key: ByteArray, value: String, now: Double = System.currentTimeMillis() / 1000.0): Session? {
        val claims = openClaims(key, value) ?: return null
        val sub = claims["sub"]
        val name = claims["name"]
        val image = claims["image"]
        val iat = claims["iat"]
        val exp = claims["exp"]
        if (sub !is String || name !is String || image !is String || !isInteger(iat) || !isInteger(exp)) return null
        if (!(now < (exp as Double)) || !accountSub.matches(sub)) return null
        return Session(sub, name, image, (iat as Double).toLong(), exp.toLong())
    }
}
