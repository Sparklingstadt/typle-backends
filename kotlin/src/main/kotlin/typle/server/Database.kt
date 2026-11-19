package typle.server

import com.zaxxer.hikari.HikariConfig
import com.zaxxer.hikari.HikariDataSource
import java.net.URI
import java.net.URLDecoder
import java.sql.Connection
import java.sql.PreparedStatement
import java.sql.ResultSet
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.TimeoutException

// src/server/database.ts: one small pool per server instance, set as the Next.js pg pool is (max 5, idle connections
// closed after 30 s, 5 s to connect and for each query, TCP keep-alive).

/** A postgres:// URL (as Neon gives it) as a JDBC URL with its user and password apart. */
data class JdbcTarget(val url: String, val user: String?, val password: String?)

object DatabaseUrl {
    /**
     * The JDBC URL of url. sslmode require, prefer and verify-ca are verify-full, as node-postgres (pg 8) treats them;
     * TLS checks the server against the JVM's trusted roots. Parameters JDBC does not know (channel_binding) are left out.
     */
    fun toJdbc(url: String): JdbcTarget {
        val uri = URI(url)
        require(uri.scheme == "postgres" || uri.scheme == "postgresql") { "DATABASE_URL is not a postgres:// URL" }
        val (user, password) = (uri.rawUserInfo ?: "").split(":", limit = 2).let {
            decode(it[0]).ifEmpty { null } to it.getOrNull(1)?.let(::decode)
        }
        val host = if (uri.host.contains(':')) "[${uri.host}]" else uri.host
        val port = if (uri.port == -1) "" else ":${uri.port}"
        val database = uri.rawPath?.removePrefix("/")?.ifEmpty { null } ?: (user ?: "postgres")
        val query = (uri.rawQuery ?: "").split('&').filter { it.isNotEmpty() }.associate {
            val (k, v) = (it.split("=", limit = 2) + "").take(2)
            decode(k) to decode(v)
        }
        val params = ArrayList<String>()
        when (query["sslmode"]) {
            null, "disable" -> params += "sslmode=disable"
            "no-verify" -> params += listOf("sslmode=require", "sslfactory=org.postgresql.ssl.NonValidatingFactory")
            else -> params += listOf("sslmode=verify-full", "sslfactory=org.postgresql.ssl.DefaultJavaSSLFactory")
        }
        query["options"]?.let { params += "options=" + java.net.URLEncoder.encode(it, Charsets.UTF_8) }
        query["application_name"]?.let { params += "ApplicationName=" + java.net.URLEncoder.encode(it, Charsets.UTF_8) }
        return JdbcTarget("jdbc:postgresql://$host$port/$database?" + params.joinToString("&"), user, password)
    }

    private fun decode(text: String) = URLDecoder.decode(text.replace("+", "%2B"), Charsets.UTF_8)
}

/** The pool of this server instance, and how its queries run. */
class Database(val source: HikariDataSource) {
    companion object {
        const val TIMEOUT_MS = 5_000L

        fun open(url: String): Database {
            val target = DatabaseUrl.toJdbc(url)
            val config = HikariConfig().apply {
                jdbcUrl = target.url
                username = target.user
                password = target.password
                maximumPoolSize = 5
                // pg keeps no idle connections it was not asked for; neither does this pool.
                minimumIdle = 0
                idleTimeout = 30_000
                connectionTimeout = TIMEOUT_MS
                // The server starts without the database; the first request opens a connection, as pg does.
                initializationFailTimeout = -1
                addDataSourceProperty("tcpKeepAlive", "true")
                addDataSourceProperty("connectTimeout", (TIMEOUT_MS / 1000).toString())
                poolName = "typle"
            }
            return Database(HikariDataSource(config))
        }

        /** From DATABASE_URL (or POSTGRES_URL); null when neither is set. */
        fun fromEnv(env: Map<String, String>): Database? {
            val url = env["DATABASE_URL"]?.ifEmpty { null } ?: env["POSTGRES_URL"]?.ifEmpty { null } ?: return null
            return open(url)
        }
    }

    /**
     * A connection for a transaction; a failure drops the connection rather than lending it out again, as pg's
     * release(true) does.
     */
    fun <T> transaction(block: (Connection) -> T): T {
        val connection = source.connection
        try {
            connection.autoCommit = false
            val result = block(connection)
            connection.commit()
            connection.autoCommit = true
            connection.close()
            return result
        } catch (error: Throwable) {
            try { connection.rollback() } catch (_: Exception) {}
            source.evictConnection(connection)
            throw error
        }
    }

    /** A query on a connection of the pool, outside a transaction. */
    fun <T> withConnection(block: (Connection) -> T): T = source.connection.use(block)

    /** Resolves once the database answers SELECT 1 within TIMEOUT_MS, connecting included (ping). */
    fun ping() {
        val executor = Executors.newSingleThreadExecutor()
        try {
            val future = executor.submit {
                source.connection.use { c -> c.prepareStatement("SELECT 1").use { it.queryTimeout = 5; it.executeQuery().close() } }
            }
            try {
                future.get(TIMEOUT_MS, TimeUnit.MILLISECONDS)
            } catch (error: TimeoutException) {
                future.cancel(true)
                throw RuntimeException("the database did not answer within $TIMEOUT_MS ms")
            } catch (error: java.util.concurrent.ExecutionException) {
                throw error.cause ?: error
            }
        } finally {
            executor.shutdownNow()
        }
    }
}

/** A statement with its parameters set in order, each query given TIMEOUT_MS as pg's query_timeout gives it. */
fun Connection.statement(sql: String, vararg params: Any?): PreparedStatement {
    val statement = prepareStatement(sql)
    statement.queryTimeout = (Database.TIMEOUT_MS / 1000).toInt()
    params.forEachIndexed { i, value ->
        when (value) {
            null -> statement.setNull(i + 1, java.sql.Types.VARCHAR)
            is String -> statement.setString(i + 1, value)
            is Long -> statement.setLong(i + 1, value)
            is Int -> statement.setInt(i + 1, value)
            is java.sql.Array -> statement.setArray(i + 1, value)
            else -> statement.setObject(i + 1, value)
        }
    }
    return statement
}

fun <T> Connection.query(sql: String, vararg params: Any?, read: (ResultSet) -> T): T =
    statement(sql, *params).use { it.executeQuery().use(read) }

fun Connection.execute(sql: String, vararg params: Any?) {
    statement(sql, *params).use { it.execute() }
}
