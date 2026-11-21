require "test_helper"

# The routes without a database: the gate, the headers every answer carries and the methods each path serves.
class RoutesTest < ActionDispatch::IntegrationTest
  test "outside the comparison every request is 404 notFound" do
    with_env("TYPLE_BACKEND_COMPARE" => nil) do
      [[:get, "/api/health"], [:get, "/api/workspace"], [:put, "/api/workspace"], [:post, "/api/workspace/records"], [:get, "/"]].each do |method, path|
        process method, path
        assert_equal 404, response.status, "#{method} #{path}"
        assert_equal "{\"error\":\"notFound\"}\n", response.body
        assert_equal "rails", response.headers["x-typle-backend"]
        assert_equal "DENY", response.headers["X-Frame-Options"]
        assert_equal "frame-ancestors 'none'; base-uri 'self'; object-src 'none'", response.headers["Content-Security-Policy"]
      end
    end
  end

  test "other methods are 405 with Allow" do
    with_env("TYPLE_BACKEND_COMPARE" => "1") do
      { "/api/health" => [%i[post put patch delete options], "GET, HEAD"],
        "/api/workspace" => [%i[post patch delete options], "GET, HEAD, PUT"],
        "/api/workspace/records" => [%i[get head put patch options], "POST, DELETE"] }.each do |path, (methods, allow)|
        methods.each do |method|
          process method, path
          assert_equal 405, response.status, "#{method} #{path}"
          assert_equal allow, response.headers["Allow"]
          assert_equal "private, no-store", response.headers["Cache-Control"]
          assert_equal "rails", response.headers["x-typle-backend"]
          assert_equal "{\"error\":\"methodNotAllowed\"}\n", response.body unless method == :head
        end
      end
    end
  end

  test "an unknown path in the comparison is 404 notFound" do
    with_env("TYPLE_BACKEND_COMPARE" => "1") do
      get "/api/other"
      assert_equal 404, response.status
      assert_equal "{\"error\":\"notFound\"}\n", response.body
      assert_equal "rails", response.headers["x-typle-backend"]
    end
  end

  test "the answers carry the headers of Next.js and no others of Rails" do
    with_env("TYPLE_BACKEND_COMPARE" => "1") do
      [[:post, "/api/health"], [:get, "/api/other"]].each do |method, path|
        process method, path, headers: { "Accept" => "text/html" }
        assert_equal "Cookie", response.headers["Vary"], "#{method} #{path}"
        assert_nil response.headers["X-XSS-Protection"]
        assert_nil response.headers["X-Permitted-Cross-Domain-Policies"]
        assert_equal "application/json; charset=utf-8", response.headers["Content-Type"]
      end
    end
  end

  test "a header with bytes beyond ASCII is read as Node reads it, not refused with a 503" do
    with_env("TYPLE_BACKEND_COMPARE" => "1", "AUTH_SECRET" => "s", "AUTH_URL" => "http://127.0.0.1:3101") do
      put "/api/workspace", params: "{}", headers: { "Origin" => "http://127.0.0.1:3101", "Content-Type" => "applicatio\xE9/json".b }
      assert_equal 403, response.status, response.body
      assert_equal "{\"error\":\"forbiddenRequest\"}\n", response.body
    end
    assert_equal "v", Typle::Sessions.cookie_value("\xC3\xA9=1; typle-session=v".b, "typle-session")
    assert_equal "ab\u00C3\u00A9", Typle::Sessions.cookie_value("typle-session=ab\xC3\xA9".b, "typle-session")
  end
end
