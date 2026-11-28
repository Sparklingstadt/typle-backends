require "base64"
require "openssl"

module Typle
  # The login cookie that Better Auth sets in typle-r (src/server/betterAuth.ts): the session token, signed with
  # BETTER_AUTH_SECRET as better-call's signCookieValue signs it. The cookie only names a session; who is signed in, and
  # until when, is the session's row in typle_auth_sessions (AuthSession.user_of). Only reading is served here: typle-r
  # signs the users in, and sign is kept for the tests. The apps' bearer tokens are gone from typle-r and not read.
  module Sessions
    # An HMAC-SHA256 in standard base64 with its padding: 44 characters, the only length better-call's getSignedCookie
    # accepts, ending in "=". Unused bits are not checked (Go's base64.StdEncoding does not either).
    SIGNATURE = %r{\A[A-Za-z0-9+/]{42}(?:[A-Za-z0-9+/]=|==)\z}

    module_function

    # The name of Better Auth's session cookie: better-auth.session_token, with the __Secure- prefix when AUTH_URL is
    # HTTPS (createCookieGetter in better-auth's cookies).
    def cookie_name(secure) = secure ? "__Secure-better-auth.session_token" : "better-auth.session_token"

    # The session token of a session cookie's value as the Cookie header carries it, as better-call's getSignedCookie
    # reads it: the value is URL-decoded (kept as it is when it cannot be, as tryDecode does), then split at its last "."
    # into the token and its signature, which must be the HMAC-SHA256 of the token under secret in standard base64. nil
    # for any other value, and for an empty secret.
    def token(value, secret)
      return nil if secret.to_s.empty?

      value = decode_uri_component(value)
      at = value.rindex(".")
      return nil if at.nil? || at < 1

      token = value[0...at]
      signature = value[(at + 1)..]
      return nil unless SIGNATURE.match?(signature)

      sent = Base64.decode64(signature)
      want = mac(token, secret)
      sent.bytesize == want.bytesize && OpenSSL.fixed_length_secure_compare(sent, want) ? token : nil
    end

    # The cookie value Better Auth sets for token (signCookieValue): the token, ".", its signature, URL-encoded as
    # encodeURIComponent encodes it.
    def sign(token, secret)
      encode_uri_component("#{token}.#{Base64.strict_encode64(mac(token, secret))}")
    end

    def mac(token, secret) = OpenSSL::HMAC.digest("SHA256", secret, token)

    # decodeURIComponent, or text as it is when that throws (a % not followed by two hex digits, or escapes that are not
    # UTF-8).
    def decode_uri_component(text)
      return text unless text.include?("%")
      return text if text.match?(/%(?!\h\h)/)

      decoded = text.b.gsub(/%\h\h/) { |escape| escape[1, 2].hex.chr }.force_encoding(Encoding::UTF_8)
      decoded.valid_encoding? ? decoded : text
    end

    # encodeURIComponent: every byte of the UTF-8 text but A-Z a-z 0-9 - _ . ! ~ * ' ( ) escaped.
    def encode_uri_component(text)
      text.b.gsub(/[^A-Za-z0-9\-_.!~*'()]/n) { |byte| format("%%%02X", byte.ord) }.force_encoding(Encoding::UTF_8)
    end

    # The value of the first cookie named name in a Cookie header, as better-call's parseCookies reads it: the first pair
    # with that name wins, and a value that starts with a double quote loses its first and last character. The value is
    # not decoded here (token does).
    def cookie_value(header, name)
      Js.header(header.to_s).split(";").each do |part|
        at = part.index("=") or next
        next unless Js.trim(part[0, at]) == name

        value = Js.trim(part[(at + 1)..])
        return value.start_with?('"') ? value[1...[1, value.length - 1].max] : value
      end
      nil
    end
  end
end
