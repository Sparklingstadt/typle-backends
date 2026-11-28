module Typle
  # The login settings as typle-r reads them: AUTH_URL as readLoginSettings of src/server/settings.ts (the app's origin),
  # BETTER_AUTH_SECRET as betterAuthSettings of src/server/authUser.ts (trimmed) and DEMO_LOGIN as demoConfigured.
  class LoginSettings
    attr_reader :origin, :secure, :secret, :demo_login

    # nil when AUTH_URL is not an http(s) URL with a host (readLoginSettings is null: 503 authUnavailable). The secret may
    # be empty: Better Auth then fails to start, which the routes that need the login answer with 503
    # databaseUnavailable.
    def self.read(env = ENV)
      origin, secure = origin_of(env["AUTH_URL"].to_s)
      return nil if origin.nil?

      new(origin, secure, Js.trim(env["BETTER_AUTH_SECRET"].to_s), env["DEMO_LOGIN"] == "1")
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

    def initialize(origin, secure, secret, demo_login)
      @origin = origin
      @secure = secure
      @secret = secret
      @demo_login = demo_login
    end

    # The login cookie's name: __Secure- over HTTPS.
    def cookie_name = Sessions.cookie_name(secure)
  end
end
