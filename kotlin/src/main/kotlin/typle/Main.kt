package typle

import io.ktor.http.ContentType
import io.ktor.http.Headers
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.content.OutgoingContent
import io.ktor.server.application.ApplicationCallPipeline
import io.ktor.server.application.call
import io.ktor.server.engine.embeddedServer
import io.ktor.server.netty.Netty
import io.ktor.server.request.httpMethod
import io.ktor.server.request.path
import io.ktor.server.request.receiveChannel
import io.ktor.server.response.respond
import io.ktor.utils.io.readRemaining
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.io.readByteArray
import typle.server.Backend
import typle.server.Request
import typle.server.Response

/** The most body bytes read: one past the largest a save may send, so a larger one is known to be too large. */
private const val BODY_LIMIT = (1L shl 20) + 1

fun main() {
    val env = System.getenv()
    val backend = Backend.fromEnv(env)
    val port = env["PORT"]?.toIntOrNull() ?: 8080
    embeddedServer(Netty, port = port, host = "0.0.0.0") {
        intercept(ApplicationCallPipeline.Call) {
            val method = call.request.httpMethod.value
            val headers = call.request.headers.entries().flatMap { entry -> entry.value.map { entry.key to it } }
            val body = if (method == "GET" || method == "HEAD") ByteArray(0)
            else call.receiveChannel().readRemaining(BODY_LIMIT).readByteArray()
            val request = Request(method, call.request.path(), headers) { body }
            // The handlers block on JDBC.
            val response = withContext(Dispatchers.IO) { backend.handle(request) }
            call.respond(outgoing(response))
            finish()
        }
    }.start(wait = true)
}

/** A handler's answer as Ktor sends it: Content-Type as the content's type, the other headers as they are. */
private fun outgoing(response: Response): OutgoingContent {
    val type = response.header(HttpHeaders.ContentType)?.let(ContentType::parse)
    val others = Headers.build {
        for ((name, value) in response.headers) if (!name.equals(HttpHeaders.ContentType, ignoreCase = true)) append(name, value)
    }
    val status = HttpStatusCode.fromValue(response.status)
    val body = response.body
    if (body == null) {
        return object : OutgoingContent.NoContent() {
            override val status = status
            override val headers = others
            override val contentType = type
        }
    }
    return object : OutgoingContent.ByteArrayContent() {
        override val status = status
        override val headers = others
        override val contentType = type
        override val contentLength = body.size.toLong()
        override fun bytes() = body
    }
}
