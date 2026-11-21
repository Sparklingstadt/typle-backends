require "uri"

module Typle
  # The database: the URL of DATABASE_URL (else POSTGRES_URL, as src/server/database.ts reads them; in the tests
  # TYPLE_TEST_DATABASE_URL), checked as node-postgres 8 checks it, with the pool of src/server/database.ts.
  # config/database.yml reads it before Rails loads the app, so it requires nothing of the app.
  module DatabaseUrl
    ROOT_CERTIFICATES = "/etc/ssl/certs/ca-certificates.crt"

    module_function

    def from_env(env = ENV, rails_env = ENV.fetch("RAILS_ENV", "development"))
      url = rails_env == "test" ? env["TYPLE_TEST_DATABASE_URL"] : env["DATABASE_URL"].to_s.then { |value| value.empty? ? env["POSTGRES_URL"] : value }
      url.nil? || url.empty? ? nil : url
    end

    def configured? = !from_env.nil?

    # url with sslmode require (or prefer, or no-verify) made verify-full: node-postgres 8 checks the server's
    # certificate for those, which libpq does only for verify-full.
    def rewrite(url)
      uri = URI.parse(url)
      query = URI.decode_www_form(uri.query.to_s)
      mode = query.assoc("sslmode")&.last
      return url unless %w[require prefer verify-ca no-verify].include?(mode)

      query = query.map { |key, value| key == "sslmode" ? [key, "verify-full"] : [key, value] }
      query << ["sslrootcert", ENV.fetch("PGSSLROOTCERT", ROOT_CERTIFICATES)] unless query.assoc("sslrootcert")
      uri.query = URI.encode_www_form(query)
      uri.to_s
    end

    # The connection's settings: at most 5 connections, an idle one closed after 30 seconds, the connection and each
    # statement given up after 5 seconds, TCP keep-alive.
    def config(url)
      {
        "adapter" => "postgresql",
        "url" => url && rewrite(url),
        "max_connections" => 5,
        "idle_timeout" => 30,
        "connect_timeout" => 5,
        "checkout_timeout" => 5,
        "keepalives" => 1,
        "variables" => { "statement_timeout" => 5000 },
      }
    end
  end
end
