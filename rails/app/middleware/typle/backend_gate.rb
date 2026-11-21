module Typle
  # The comparison's gate and the headers of every answer (kotlin/src/main/kotlin/typle/server/Routes.kt): outside the
  # comparison (TYPLE_BACKEND_COMPARE is not 1) every request is 404, so a deployment without it reads and writes
  # nothing. Every answer carries the headers next.config.mjs adds (and its CSP of /api) and x-typle-backend: rails.
  class BackendGate
    BACKEND_HEADER = "x-typle-backend"
    SECURITY_HEADERS = {
      "x-frame-options" => "DENY",
      "x-content-type-options" => "nosniff",
      "referrer-policy" => "strict-origin-when-cross-origin",
      "permissions-policy" => "camera=(), microphone=(), geolocation=()",
      "content-security-policy" => "frame-ancestors 'none'; base-uri 'self'; object-src 'none'",
    }.freeze
    NOT_FOUND = "{\"error\":\"notFound\"}\n".freeze

    def initialize(app)
      @app = app
    end

    # Where the Content-Type header is kept for the handlers, which check it as src/server/request.ts does. Rails sees
    # application/octet-stream instead, so it neither reads the body (as a form, which Rack takes a body without a type
    # for) nor refuses a header it cannot read as a MIME type.
    CONTENT_TYPE = "typle.content_type"
    OPAQUE = "application/octet-stream"

    def call(env)
      env[CONTENT_TYPE] = env["CONTENT_TYPE"]
      env["CONTENT_TYPE"] = OPAQUE
      status, headers, body = ENV["TYPLE_BACKEND_COMPARE"] == "1" ? @app.call(env) : not_found
      headers.merge!(SECURITY_HEADERS)
      headers[BACKEND_HEADER] = "rails"
      [status, headers, body]
    end

    private

    def not_found
      [404, Rack::Headers[
        "content-type" => "application/json; charset=utf-8",
        "cache-control" => "private, no-store",
        "vary" => "Cookie",
      ], [NOT_FOUND]]
    end
  end
end
