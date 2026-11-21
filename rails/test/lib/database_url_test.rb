require "test_helper"

class DatabaseUrlTest < ActiveSupport::TestCase
  test "a Neon URL is checked as node-postgres checks it: sslmode=require is verify-full" do
    url = Typle::DatabaseUrl.rewrite("postgresql://user:p%40ss@ep-x.ap-southeast-1.aws.neon.tech/neondb?sslmode=require&channel_binding=require")
    query = Rack::Utils.parse_query(URI.parse(url).query)
    assert_equal "verify-full", query["sslmode"]
    assert_equal "require", query["channel_binding"]
    assert_equal "/etc/ssl/certs/ca-certificates.crt", query["sslrootcert"]
    assert_equal "p%40ss", URI.parse(url).password
  end

  test "a local URL is left as it is, and the pool is the one of Next.js" do
    config = Typle::DatabaseUrl.config("postgres://typle:x@127.0.0.1:5433/typle")
    assert_equal "postgres://typle:x@127.0.0.1:5433/typle", config["url"]
    assert_equal 5, config["max_connections"]
    assert_equal 30, config["idle_timeout"]
    assert_equal 5, config["connect_timeout"]
    assert_equal({ "statement_timeout" => 5000 }, config["variables"])
  end

  test "the URL is DATABASE_URL, else POSTGRES_URL, and TYPLE_TEST_DATABASE_URL in the tests" do
    assert_equal "postgres://a", Typle::DatabaseUrl.from_env({ "DATABASE_URL" => "postgres://a", "POSTGRES_URL" => "postgres://b" }, "production")
    assert_equal "postgres://b", Typle::DatabaseUrl.from_env({ "DATABASE_URL" => "", "POSTGRES_URL" => "postgres://b" }, "production")
    assert_equal "postgres://t", Typle::DatabaseUrl.from_env({ "DATABASE_URL" => "postgres://a", "TYPLE_TEST_DATABASE_URL" => "postgres://t" }, "test")
    assert_nil Typle::DatabaseUrl.from_env({}, "production")
  end
end
