require "test_helper"

# The login as server/internal/web/sessions_test.go checks it, without a database: the sessions and workspaces are
# stubbed (a token "session-of:<owner>" is a session of owner), so only the reading of the cookie and the answers are
# checked here. workspace_test.rb reads real session rows when TYPLE_TEST_DATABASE_URL is set.
class SessionsIntegrationTest < ActionDispatch::IntegrationTest
  SECRET = "test secret"
  ORIGIN = "http://127.0.0.1:3101"
  OWNER = "github:1"
  ENV_KEYS = %w[TYPLE_BACKEND_COMPARE AUTH_URL BETTER_AUTH_SECRET DEMO_LOGIN].freeze

  setup do
    @saved_env = ENV_KEYS.to_h { |key| [key, ENV[key]] }
    ENV.update("TYPLE_BACKEND_COMPARE" => "1", "AUTH_URL" => ORIGIN, "BETTER_AUTH_SECRET" => SECRET)
    ENV.delete("DEMO_LOGIN")
    @reports = []
    @asked_at = []
  end

  teardown do
    @saved_env.each { |key, value| value.nil? ? ENV.delete(key) : ENV[key] = value }
  end

  def token_of(owner) = "session-of:#{owner}"

  # Better Auth's session cookie for a session of owner, as a Cookie header.
  def cookie(owner, secret: SECRET, secure: false)
    "#{Typle::Sessions.cookie_name(secure)}=#{Typle::Sessions.sign(token_of(owner), secret)}"
  end

  def fake_user_of(token, now)
    @asked_at << now
    owner = token.delete_prefix("session-of:")
    owner == token ? nil : Typle::Session.new(owner, "", "")
  end

  # Runs the block with a database said to be there, the stubbed sessions and an empty workspace that saves anything.
  def with_fakes(user_of: method(:fake_user_of), &block)
    saved = Typle::WorkspaceStore::Saved.new([], nil, [], nil, 2)
    Typle::DatabaseUrl.stub(:configured?, true) do
      AuthSession.stub(:user_of, user_of) do
        Typle::WorkspaceStore.stub(:read, nil) do
          Typle::WorkspaceStore.stub(:write, saved) do
            Typle::Report.stub(:failure, ->(_, _, operation) { @reports << operation }, &block)
          end
        end
      end
    end
  end

  def read(headers = {}) = get("/api/workspace", headers: headers)

  def save(headers = {})
    put "/api/workspace", params: %({"revision":1,"accountId":"#{OWNER}","devices":[]}),
                          headers: { "Origin" => ORIGIN, "Content-Type" => "application/json", "Cookie" => cookie(OWNER) }.merge(headers).compact
  end

  def expect_error(status, key)
    assert_equal status, response.status, response.body
    assert_equal "{\"error\":\"#{key}\"}\n", response.body
  end

  test "a Better Auth session opens the account's data, and no answer sets a cookie" do
    with_fakes do
      read("Cookie" => cookie(OWNER))
      assert_equal 200, response.status, response.body
      assert_equal OWNER, Typle::JsonValue.parse(response.body)["accountId"]
      save
      assert_equal 200, response.status, response.body
      read
      expect_error 401, "loginRequired"
      assert_nil response.headers["Set-Cookie"]
    end
  end

  test "over HTTPS the cookie is the __Secure- one, and the plain one is no one's" do
    with_env("AUTH_URL" => "https://typle.example") do
      with_fakes do
        read("Cookie" => cookie(OWNER, secure: true))
        assert_equal 200, response.status, response.body
        read("Cookie" => cookie(OWNER, secure: false))
        expect_error 401, "loginRequired"
      end
    end
  end

  test "a cookie that is not a session's is loginRequired" do
    with_fakes do
      {
        "Auth.js cookie" => "authjs.session-token=x",
        "tampered" => cookie(OWNER).sub(/.%3D\z/) { |tail| (tail[0] == "A" ? "B" : "A") + "%3D" },
        "other secret" => cookie(OWNER, secret: "another secret"),
        "__Secure- over http" => cookie(OWNER, secure: true),
        "no such session" => "better-auth.session_token=#{Typle::Sessions.sign("gone", SECRET)}",
      }.each do |name, header|
        read("Cookie" => header)
        assert_equal 401, response.status, name
        assert_nil response.headers["Set-Cookie"], name
      end
    end
  end

  # The iPad and Mac apps' bearer tokens are gone from typle-r: an Authorization header neither opens the data nor
  # stands in for the cookie's Origin.
  test "an Authorization header is ignored" do
    with_fakes do
      read("Authorization" => "Bearer x")
      expect_error 401, "loginRequired"
      read("Authorization" => "Bearer x", "Cookie" => cookie(OWNER))
      assert_equal 200, response.status, response.body
      save("Origin" => nil, "Authorization" => "Bearer x")
      expect_error 403, "forbiddenRequest"
    end
  end

  test "when the sessions cannot be read, the data is not served" do
    with_fakes(user_of: ->(*) { raise "down" }) do
      read("Cookie" => cookie(OWNER))
      expect_error 503, "databaseUnavailable"
    end
    assert_equal ["session.check"], @reports
  end

  # Without BETTER_AUTH_SECRET Better Auth does not start in typle-r, and reading the session throws.
  test "without BETTER_AUTH_SECRET every request that needs the login is 503 databaseUnavailable" do
    with_env("BETTER_AUTH_SECRET" => " ") do
      with_fakes do
        read("Cookie" => cookie(OWNER))
        expect_error 503, "databaseUnavailable"
        read
        expect_error 503, "databaseUnavailable"
      end
    end
    assert_equal ["session.check", "session.check"], @reports
  end

  test "the shared demo account's sessions work only while DEMO_LOGIN is 1" do
    with_fakes do
      read("Cookie" => cookie("demo:shared"))
      expect_error 401, "loginRequired"
      with_env("DEMO_LOGIN" => "1") do
        read("Cookie" => cookie("demo:shared"))
        assert_equal 200, response.status, response.body
      end
    end
  end

  test "the cookie is the first of its name, a leading quote dropped with the last character" do
    name = Typle::Sessions.cookie_name(false)
    good = cookie(OWNER).delete_prefix("#{name}=")
    {
      "#{name}=#{good}" => 200,
      "a=b; #{name}=\"#{good}\"" => 200,
      " #{name} = #{good} ;x=y" => 200,
      "#{name}=bad; #{name}=#{good}" => 401,
      "__Secure-#{name}=#{good}" => 401,
      "better-auth.session_data=#{good}" => 401,
      "" => 401,
    }.each do |header, status|
      with_fakes { read("Cookie" => header) }
      assert_equal status, response.status, "Cookie #{header.inspect}"
    end
  end

  test "the session is looked up at the time of the request" do
    before = Time.now
    with_fakes { read("Cookie" => cookie(OWNER)) }
    assert_equal 200, response.status
    assert_equal 1, @asked_at.size
    assert_operator @asked_at[0], :>=, before
    assert_operator @asked_at[0], :<=, Time.now
  end
end
