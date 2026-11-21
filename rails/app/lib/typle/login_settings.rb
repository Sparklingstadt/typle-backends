require "openssl"

module Typle
  # src/server/settings.ts: the app's origin and the key of the login cookie, derived from AUTH_SECRET.
  class LoginSettings
    SESSION_INFO = "typle session v1"

    attr_reader :origin, :session_key, :secure

    # HKDF-SHA256 of secret with no salt, 32 bytes, as node:crypto's hkdfSync derives it.
    def self.derive_key(secret, info)
      OpenSSL::KDF.hkdf(secret, salt: "", info: info, length: 32, hash: "SHA256")
    end

    # readLoginSettings: nil when AUTH_SECRET is empty or AUTH_URL is not an http(s) URL with a host.
    def self.read(env = ENV)
      secret = env["AUTH_SECRET"].to_s
      origin, secure = origin_of(env["AUTH_URL"].to_s)
      return nil if origin.nil? || secret.empty?

      new(origin, derive_key(secret, SESSION_INFO), secure)
    end

    # The origin of an http or https URL as new URL(text).origin gives it, and whether it is https.
    def self.origin_of(text)
      uri = URI.parse(text.gsub(/\A[\x00-\x20]+|[\x00-\x20]+\z/, ""))
      scheme = uri.scheme&.downcase
      host = uri.host&.downcase
      return nil unless %w[http https].include?(scheme) && host.present?

      port = uri.port.nil? || uri.port == uri.default_port ? "" : ":#{uri.port}"
      ["#{scheme}://#{host}#{port}", scheme == "https"]
    rescue URI::Error
      nil
    end

    def initialize(origin, session_key, secure)
      @origin = origin
      @session_key = session_key
      @secure = secure
    end

    # The login cookie's name: __Host- over HTTPS.
    def cookie_name = secure ? "__Host-typle-session" : "typle-session"
  end
end
