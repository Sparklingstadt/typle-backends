require "test_helper"

# The endpoints as kotlin/src/test/kotlin/typle/ServerTest.kt checks them: without a database first, then the scenarios
# the Go backend ran against a real database (TYPLE_TEST_DATABASE_URL; skipped when it is not set).
class WorkspaceTest < ActionDispatch::IntegrationTest
  SECRET = "typle-rails-test-secret"
  ORIGIN = "http://127.0.0.1:3101"
  AT = "2026-09-07T00:00:00.000Z"
  OWNERS = Concurrent::Array.new

  Minitest.after_run do
    next if OWNERS.empty? || !Typle::DatabaseUrl.configured?

    %w[typle_session_revocations typle_records typle_workspaces].each do |table|
      ActiveRecord::Base.connection.exec_delete(ActiveRecord::Base.sanitize_sql(["DELETE FROM #{table} WHERE owner_id IN (?)", OWNERS.to_a]))
    end
  end

  setup do
    @saved_env = %w[TYPLE_BACKEND_COMPARE AUTH_SECRET AUTH_URL].to_h { |key| [key, ENV[key]] }
    ENV.update("TYPLE_BACKEND_COMPARE" => "1", "AUTH_SECRET" => SECRET, "AUTH_URL" => ORIGIN)
  end

  teardown do
    @saved_env.each { |key, value| value.nil? ? ENV.delete(key) : ENV[key] = value }
  end

  def database!
    skip "TYPLE_TEST_DATABASE_URL is not set" unless Typle::DatabaseUrl.configured?
  end

  def new_owner = "github:#{rand(1_000_000_000...2_000_000_000)}".tap { |owner| OWNERS << owner }

  def seal(owner, iat: Time.now.to_i - 60)
    key = Typle::LoginSettings.derive_key(SECRET, "typle session v1")
    payload = Base64.urlsafe_encode64(%({"sub":"#{owner}","name":"N","image":"","iat":#{iat},"exp":#{Time.now.to_i + 3600}}), padding: false)
    "#{payload}.#{Base64.urlsafe_encode64(OpenSSL::HMAC.digest("SHA256", key, payload), padding: false)}"
  end

  def headers_for(owner, extra = {})
    headers = { "Origin" => ORIGIN, "Content-Type" => "application/json" }
    headers["Cookie"] = "typle-session=#{seal(owner)}" if owner
    headers.merge(extra).reject { |_, value| value.nil? }
  end

  def library(name)
    %("packs":[{"id":"p","name":"P","createdAt":"#{AT}"}],"lists":[{"id":"l","name":"#{name}","packId":"p","words":[{"display":"a","input":"a","annotation":""}],"records":[],"createdAt":"#{AT}"}])
  end

  def expect_error(status, key)
    assert_equal status, response.status, response.body
    assert_equal "{\"error\":\"#{key}\"}\n", response.body
  end

  def answer = Typle::JsonValue.parse(response.body)

  def json(value) = Typle::JsonValue.parse(Typle::JsonValue.generate(value))

  def insert_row(owner, row)
    Workspace.insert_all([{ owner_id: owner, lists: row["lists"], packs: row["packs"], devices: row["devices"],
                            interests: row["interests"], revision: row["revision"], records_split: row["split"] }], record_timestamps: false)
  end

  def state(owner)
    connection = ActiveRecord::Base.connection
    row = connection.select_value(ActiveRecord::Base.sanitize_sql([<<~SQL, owner]))
      SELECT COALESCE((SELECT jsonb_build_object('lists', lists, 'packs', packs, 'devices', devices, 'interests', interests,
        'revision', revision, 'split', records_split) FROM typle_workspaces WHERE owner_id = ?), 'null'::jsonb)::text
    SQL
    records = connection.select_value(ActiveRecord::Base.sanitize_sql([<<~SQL, owner]))
      SELECT COALESCE(jsonb_agg(jsonb_build_array(list_id, record_id, position, (extract(epoch FROM at) * 1000)::bigint, data)
        ORDER BY list_id, position), '[]'::jsonb)::text FROM typle_records WHERE owner_id = ?
    SQL
    { "row" => Typle::JsonValue.parse(row), "records" => Typle::JsonValue.parse(records) }
  end

  # A save of body as /api/workspace answers it: ok, or the key it is refused with.
  def save(owner, body)
    read = Typle::WorkspaceChanges.read(body.merge("accountId" => owner))
    return read[1] if read.first == :refused

    Typle::WorkspaceStore.write(owner, read[1], read[2])
    "ok"
  rescue Typle::Conflict
    "conflict"
  rescue Typle::AppError => e
    e.key
  end

  # Without a database

  test "without a database the health is 503, and a write is refused before it is opened" do
    with_env("TYPLE_TEST_DATABASE_URL" => nil) do
      get "/api/health"
      expect_error 503, "databaseUnavailable"
      head "/api/health"
      assert_equal 503, response.status
      assert_empty response.body
      put "/api/workspace", params: "{}", headers: { "Origin" => "https://evil.example", "Content-Type" => "application/json" }
      expect_error 403, "forbiddenRequest"
      post "/api/workspace/records", params: "{}", headers: { "Origin" => ORIGIN, "Content-Type" => "Application/JSON" }
      expect_error 403, "forbiddenRequest"
      get "/api/workspace"
      expect_error 503, "databaseUnavailable"
      with_env("AUTH_SECRET" => nil) do
        get "/api/workspace"
        expect_error 503, "authUnavailable"
      end
    end
  end

  test "an app's bearer token is not a login" do
    with_env("TYPLE_TEST_DATABASE_URL" => nil) do
      put "/api/workspace", params: "{}", headers: { "Authorization" => "Bearer x", "Content-Type" => "application/json" }
      expect_error 403, "forbiddenRequest"
    end
  end

  # With a database

  test "workspace writes as the Go backend (workspace-writes.json)" do
    database!
    Fixtures.read("workspace-writes.json")["scenarios"].each do |scenario|
      owner = new_owner
      row = scenario.dig("setup", "row")
      insert_row(owner, row) if row
      scenario["steps"].each_with_index do |step, index|
        message = "#{scenario["name"]}: step #{index + 1}"
        assert_equal step["result"], save(owner, step["body"]), message
        assert_equal json({ "row" => step["row"], "records" => step["records"] }), state(owner), message
      end
    end
  end

  test "record writes as the Go backend (record-writes.json)" do
    database!
    Fixtures.read("record-writes.json")["scenarios"].each do |scenario|
      owner = new_owner
      row = scenario.dig("setup", "row")
      insert_row(owner, row) if row
      scenario["steps"].each_with_index do |step, index|
        message = "#{scenario["name"]}: step #{index + 1}"
        body = { "accountId" => owner }.merge(step["body"])
        process(step["kind"] == "add" ? :post : :delete, "/api/workspace/records", params: Typle::JsonValue.generate(body), headers: headers_for(owner))
        assert_equal step["status"], response.status, "#{message}: #{response.body}"
        assert_equal({ "error" => step["key"] }, answer, message) unless step["key"] == ""
        assert_equal json(step["record"]), answer["record"], message if step["kind"] == "add" && step["key"] == ""
        assert_equal json({ "row" => step["row"], "records" => step["records"] }), state(owner), message
      end
    end
  end

  test "an account that has saved nothing reads as revision 0, and a save is read back" do
    database!
    owner = new_owner
    get "/api/workspace", headers: headers_for(owner)
    assert_equal 200, response.status
    assert_equal({ "lists" => [], "packs" => nil, "devices" => [], "interests" => Typle::Interests::DEFAULT_INTERESTS,
                   "revision" => 0, "accountId" => owner }, answer)
    put "/api/workspace", params: %({"revision":0,"accountId":"#{owner}",#{library("one")}}), headers: headers_for(owner)
    assert_equal 200, response.status, response.body
    assert_equal 1, answer["revision"]
    saved = response.body
    get "/api/workspace", headers: headers_for(owner)
    assert_equal saved, response.body
    put "/api/workspace", params: %({"revision":0,"accountId":"#{owner}",#{library("two")}}), headers: headers_for(owner)
    expect_error 409, "conflict"
    # PUT always writes: the same workspace sent back at its revision moves the revision on.
    put "/api/workspace", params: %({"revision":1,"accountId":"#{owner}",#{library("one")}}), headers: headers_for(owner)
    assert_equal 2, answer["revision"]
  end

  test "a save is refused as the Go backend refused it" do
    database!
    owner = new_owner
    good = %({"revision":0,"accountId":"#{owner}",#{library("one")}})
    put "/api/workspace", params: good, headers: headers_for(owner, "Origin" => "https://evil.example")
    expect_error 403, "forbiddenRequest"
    put "/api/workspace", params: good, headers: headers_for(owner, "Content-Type" => "text/plain")
    expect_error 403, "forbiddenRequest"
    put "/api/workspace", params: good, headers: headers_for(nil)
    expect_error 401, "loginRequired"
    put "/api/workspace", params: "", headers: headers_for(owner)
    expect_error 400, "emptyRequest"
    put "/api/workspace", params: %({"x":"#{"a" * (1 << 20)}"}), headers: headers_for(owner)
    expect_error 413, "workspaceTooLarge"
    put "/api/workspace", params: "[]", headers: headers_for(owner)
    expect_error 400, "invalidWorkspace"
    put "/api/workspace", params: "{", headers: headers_for(owner)
    expect_error 400, "invalidWorkspace"
    put "/api/workspace", params: %({"revision":0,"accountId":"#{owner}","lists":[{"id":"a","name":"a\\ud800"}]}), headers: headers_for(owner)
    expect_error 400, "invalidWorkspace"
    put "/api/workspace", params: %({"revision":0,"accountId":"github:2",#{library("one")}}), headers: headers_for(owner)
    expect_error 401, "loginAccountChanged"
  end

  test "a revoked login is refused, and a browser loses its cookie" do
    database!
    owner = new_owner
    SessionRevocation.insert_all([{ owner_id: owner, revoked_at: Time.now }])
    get "/api/workspace", headers: headers_for(owner)
    expect_error 401, "loginRequired"
    assert_equal "typle-session=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax", response.headers["Set-Cookie"]
  end

  test "of two saves from the same revision, only one is saved" do
    database!
    owner = new_owner
    body = ->(revision, name) { Typle::JsonValue.parse(%({"revision":#{revision},#{library(name)}})) }
    run = ->(revision) { [Thread.new { save(owner, body.(revision, "one")) }, Thread.new { save(owner, body.(revision, "two")) }].map(&:value).sort }
    assert_equal %w[conflict ok], run.(0)
    assert_equal %w[conflict ok], run.(1)
    assert_equal 2, state(owner)["row"]["revision"]
  end

  test "the health answers 200 once the database answers" do
    database!
    get "/api/health"
    assert_equal 200, response.status
    assert_equal "{\"status\":\"ok\"}\n", response.body
    assert_equal "private, no-store", response.headers["Cache-Control"]
  end

  # Review focus

  test "integral floats are saved and answered as JS writes them" do
    database!
    owner = new_owner
    record = %({"id":"r","time":1500.0,"date":"#{AT}","mistakes":0,"accuracy":100.0,"wordCount":1})
    put "/api/workspace", params: %({"revision":0,"accountId":"#{owner}",#{library("one").sub('"records":[]', %("records":[#{record}]))}}), headers: headers_for(owner)
    assert_equal 200, response.status, response.body
    assert_includes response.body, '"time":1500,'
    assert_equal '{"id": "r", "date": "2026-09-07T00:00:00.000Z", "time": 1500, "accuracy": 100, "mistakes": 0, "wordCount": 1}',
                 ActiveRecord::Base.connection.select_value(ActiveRecord::Base.sanitize_sql(["SELECT data::text FROM typle_records WHERE owner_id = ?", owner]))
  end

  test "a body of exactly 1 MiB is read, one byte more is 413" do
    database!
    owner = new_owner
    put "/api/workspace", params: %({"x":"#{"a" * ((1 << 20) - 8)}"}), headers: headers_for(owner)
    expect_error 401, "loginAccountChanged"
    put "/api/workspace", params: %({"x":"#{"a" * ((1 << 20) - 7)}"}), headers: headers_for(owner)
    expect_error 413, "workspaceTooLarge"
  end

  test "a body is read as TextDecoder reads it: the BOM dropped, malformed bytes as U+FFFD" do
    database!
    owner = new_owner
    put "/api/workspace", params: "\xEF\xBB\xBF{\"revision\":0,\"accountId\":\"#{owner}\",#{library("one")}}".b, headers: headers_for(owner)
    assert_equal 200, response.status, response.body
    put "/api/workspace", params: "{\"revision\":1,\"accountId\":\"#{owner}\",#{library("o\xFFe")}}".b, headers: headers_for(owner)
    assert_equal 200, response.status, response.body
    assert_equal "o�e", answer["lists"][0]["name"]
  end

  test "an unexpected error is a JSON 503 with the headers" do
    database!
    owner = new_owner
    Typle::WorkspaceStore.stub(:write, ->(*) { raise "boom" }) do
      put "/api/workspace", params: %({"revision":0,"accountId":"#{owner}",#{library("one")}}), headers: headers_for(owner)
    end
    expect_error 503, "workspaceSaveFailed"
    assert_equal "rails", response.headers["x-typle-backend"]
    Typle::WorkspaceStore.stub(:read, ->(*) { raise "boom" }) do
      get "/api/workspace", headers: headers_for(owner)
    end
    expect_error 503, "databaseUnavailable"
  end

  test "each request sends the statements the Next.js handlers send" do
    database!
    owner = new_owner
    put "/api/workspace", params: %({"revision":0,"accountId":"#{owner}",#{library("one")}}), headers: headers_for(owner)
    counted = lambda do |&block|
      statements = []
      callback = ->(*, payload) { statements << payload[:sql] unless %w[SCHEMA TRANSACTION].include?(payload[:name]) || payload[:sql] =~ /\A(BEGIN|COMMIT|ROLLBACK)/ }
      ActiveSupport::Notifications.subscribed(callback, "sql.active_record", &block)
      statements
    end
    # The revocation check, then the workspace with its records.
    assert_equal 2, counted.() { get "/api/workspace", headers: headers_for(owner) }.size
    # The check, the revision and counts, the update, the records' delete and count (no rows to upsert), the read back.
    put_statements = counted.() { put "/api/workspace", params: %({"revision":1,"accountId":"#{owner}",#{library("one")}}), headers: headers_for(owner) }
    assert_equal 6, put_statements.size, put_statements.join("\n")
    # The check, the row held for update, its records, the update, the records' delete, upsert and count, the read back.
    add = %({"accountId":"#{owner}","listId":"l","record":{"id":"r1","time":1,"date":"#{AT}","mistakes":0,"accuracy":100}})
    add_statements = counted.() { post "/api/workspace/records", params: add, headers: headers_for(owner) }
    assert_equal 201, response.status, response.body
    assert_equal 8, add_statements.size, add_statements.join("\n")
  end
end
