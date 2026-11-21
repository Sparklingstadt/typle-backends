require "test_helper"

# The login cookie as src/server/session.ts seals it (and the Go backend sealed it): the fixtures it left open here.
class SessionsTest < ActiveSupport::TestCase
  def settings(secret, url = "https://typle.example")
    Typle::LoginSettings.read("AUTH_SECRET" => secret, "AUTH_URL" => url)
  end

  test "the Go backend's cookies open as they did there (sessions.json)" do
    fixture = Fixtures.read("sessions.json")
    login = settings(fixture["secret"])
    now = fixture["now"]
    fixture["cookies"].each do |entry|
      session = Typle::Sessions.open(login.session_key, entry["value"], now)
      if entry["session"].nil?
        assert_nil session, entry["value"]
      else
        assert_equal entry["session"], { "sub" => session.sub, "name" => session.name, "image" => session.image,
                                         "iat" => session.issued_at, "exp" => session.expires }, entry["value"]
      end
    end
    cookie = fixture["cookies"][0]["value"]
    assert_nil Typle::Sessions.open(settings("another-secret").session_key, cookie, now)
    payload, signature = cookie.split(".")
    flip = ->(text) { text[0...-1] + (text[-1] == "A" ? "B" : "A") }
    ["#{payload}.#{flip.(signature)}", "#{flip.(payload)}.#{signature}", "#{payload}.#{signature}=", "#{payload}=.#{signature}",
     payload, "", ".", "#{payload}.#{signature}.x", "#{payload}.#{signature[0...-1]}"].each do |changed|
      assert_nil Typle::Sessions.open(login.session_key, changed, now), changed
    end
  end

  test "a value sealed with the apps' key is not a login cookie (auth-answers.json)" do
    auth = Fixtures.read("auth-answers.json")
    login = settings(auth["secret"])
    values = auth["values"]
    assert_equal "github:9100000001", Typle::Sessions.open(login.session_key, values["alice"], auth["now"])&.sub
    assert_equal "github:9100000002", Typle::Sessions.open(login.session_key, values["bob"], auth["now"])&.sub
    assert_nil Typle::Sessions.open(login.session_key, values["alice-app"], auth["now"])
  end

  test "the cookie's name follows AUTH_URL, and settings need a secret and an http(s) URL" do
    assert_equal "__Host-typle-session", settings("s").cookie_name
    assert_equal "typle-session", settings("s", "http://127.0.0.1:3101").cookie_name
    assert_equal "http://127.0.0.1:3101", settings("s", "http://127.0.0.1:3101/x").origin
    assert_equal "https://typle.example", settings("s", "HTTPS://Typle.example:443/").origin
    assert_nil settings("")
    assert_nil settings("s", "ftp://typle.example")
    assert_nil settings("s", "")
  end

  test "the cookie is read from the Cookie header as the cookies of Next.js read it" do
    assert_equal "v", Typle::Sessions.cookie_value("a=1; typle-session=v; typle-session=w", "typle-session")
    assert_equal "v", Typle::Sessions.cookie_value(" typle-session = \"v\" ", "typle-session")
    assert_nil Typle::Sessions.cookie_value("other=1", "typle-session")
    assert_nil Typle::Sessions.cookie_value(nil, "typle-session")
  end
end
