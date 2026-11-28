require "test_helper"

# Better Auth's session cookie as better-call signs it (server/internal/session/session_test.go checks the same values),
# and the login settings of the environment.
class SessionsTest < ActiveSupport::TestCase
  # Values better-call's signCookieValue (better-call 1.x in typle-r v2.68.1) signed with this secret.
  SECRET = "typle-fixture-secret"
  SIGNED = [
    ["Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj", "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D"],
    ["a.b+c/d=e", "a.b%2Bc%2Fd%3De.QgtT95hjCvQfprQC%2BUtI5LsqCC549MK9KWYHFNPmUuE%3D"],
  ].freeze

  def settings(url = "https://typle.example", secret: "s", demo: nil)
    Typle::LoginSettings.read("AUTH_URL" => url, "BETTER_AUTH_SECRET" => secret, "DEMO_LOGIN" => demo)
  end

  test "a token is signed as better-call signs it" do
    SIGNED.each { |token, value| assert_equal value, Typle::Sessions.sign(token, SECRET), token }
  end

  test "a signed value opens to its token" do
    SIGNED.each { |token, value| assert_equal token, Typle::Sessions.token(value, SECRET), value }
    # The value as it is, not URL-encoded, opens too: decoding leaves it as it is.
    assert_equal SIGNED[0][0], Typle::Sessions.token("Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK+RyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w=", SECRET)
    # A value that is not valid percent-encoding is read as it is (tryDecode).
    assert_nil Typle::Sessions.token("%zz.#{SIGNED[0][1]}", SECRET)
  end

  test "any other value is refused" do
    good = SIGNED[0][1]
    {
      "empty" => "",
      "no dot" => "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj",
      "dot first" => ".HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D",
      "other token" => "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHk.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D",
      "short" => "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w",
      "no padding end" => "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2wA",
      "not base64" => "Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK-RyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D",
    }.each { |name, value| assert_nil Typle::Sessions.token(value, SECRET), name }
    assert_nil Typle::Sessions.token(good, "another secret")
    assert_nil Typle::Sessions.token(good, "")
  end

  test "the cookie's name follows AUTH_URL" do
    assert_equal "better-auth.session_token", Typle::Sessions.cookie_name(false)
    assert_equal "__Secure-better-auth.session_token", Typle::Sessions.cookie_name(true)
    assert_equal "__Secure-better-auth.session_token", settings.cookie_name
    assert_equal "better-auth.session_token", settings("http://127.0.0.1:3101").cookie_name
  end

  test "the settings need an http(s) URL; the secret is trimmed and DEMO_LOGIN must be exactly 1" do
    assert_equal "http://127.0.0.1:3101", settings("http://127.0.0.1:3101/x").origin
    assert_equal "https://typle.example", settings("HTTPS://Typle.example:443/").origin
    assert_nil settings("ftp://typle.example")
    assert_nil settings("")
    assert_equal "", settings(secret: nil).secret
    assert_equal "", settings(secret: " \n").secret
    assert_equal "a b", settings(secret: " a b\t").secret
    assert settings(demo: "1").demo_login
    assert_not settings(demo: "true").demo_login
    assert_not settings.demo_login
  end

  test "the cookie is read from the Cookie header as better-call's parseCookies reads it" do
    name = "better-auth.session_token"
    assert_equal "v", Typle::Sessions.cookie_value("a=1; #{name}=v; #{name}=w", name)
    assert_equal "v", Typle::Sessions.cookie_value(" #{name} = \"v\" ", name)
    # A leading quote drops the first and the last character, whatever the last one is.
    assert_equal "a", Typle::Sessions.cookie_value("#{name}=\"ab", name)
    assert_equal "", Typle::Sessions.cookie_value("#{name}=\"", name)
    # The value is not decoded here.
    assert_equal "a%2Bb", Typle::Sessions.cookie_value("#{name}=a%2Bb", name)
    assert_nil Typle::Sessions.cookie_value("__Secure-#{name}=v", name)
    assert_nil Typle::Sessions.cookie_value("other=1", name)
    assert_nil Typle::Sessions.cookie_value(nil, name)
  end
end
