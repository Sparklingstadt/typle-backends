require "base64"
require "openssl"

module Typle
  # src/server/session.ts: the login cookie, base64url(JSON) + "." + base64url(HMAC-SHA256 of that text). The apps'
  # bearer tokens are not read: Next.js has not read them since v2.55.0.
  module Sessions
    ACCOUNT_SUB = /\A(github:\d+|password:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|ffpf:[a-z0-9]{1,64})\z/
    BASE64URL = /\A[A-Za-z0-9_-]*\z/

    module_function

    # The bytes of strictly written base64url text (no padding, unused bits zero), or nil.
    def strict_base64url(text)
      return nil if !BASE64URL.match?(text) || text.length % 4 == 1

      bytes = Base64.urlsafe_decode64(text + ("=" * (-text.length % 4)))
      Base64.urlsafe_encode64(bytes, padding: false) == text ? bytes : nil
    rescue ArgumentError
      nil
    end

    def open_claims(key, value)
      parts = value.split(".", -1)
      return nil unless parts.size == 2

      payload, signature = parts
      got = strict_base64url(signature)
      want = OpenSSL::HMAC.digest("SHA256", key, payload)
      return nil if got.nil? || got.bytesize != want.bytesize || !OpenSSL.fixed_length_secure_compare(got, want)

      data = strict_base64url(payload) or return nil
      # Buffer.toString('utf8') replaces malformed bytes with U+FFFD.
      claims = JsonValue.parse(data.force_encoding(Encoding::UTF_8).scrub("�"))
      claims.is_a?(Hash) ? claims : nil
    rescue JSON::ParserError
      nil
    end

    # The session in value if it was sealed with key, has not expired at now, and is for an account of this app.
    def open(key, value, now = Time.now.to_f)
      claims = open_claims(key, value) or return nil
      sub, name, image, iat, exp = claims.values_at("sub", "name", "image", "iat", "exp")
      return nil unless sub.is_a?(String) && name.is_a?(String) && image.is_a?(String) && Js.integer?(iat) && Js.integer?(exp)
      return nil unless now < exp && ACCOUNT_SUB.match?(sub)

      Session.new(sub, name, image, iat.to_i, exp.to_i)
    end

    # The value of the cookie name in a Cookie header, the first one when there are several; quotes around it dropped.
    def cookie_value(header, name)
      Js.header(header.to_s).split(";").each do |part|
        at = part.index("=") or next
        next unless Js.trim(part[0, at]) == name

        value = Js.trim(part[(at + 1)..])
        return value.length >= 2 && value.start_with?('"') && value.end_with?('"') ? value[1...-1] : value
      end
      nil
    end
  end
end
