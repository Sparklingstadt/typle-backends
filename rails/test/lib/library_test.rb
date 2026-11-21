require "test_helper"

# The checks and changes of src/lib, held to the shared fixtures as kotlin/src/test/kotlin/typle/LibTest.kt holds them.
class LibraryTest < ActiveSupport::TestCase
  def json(value) = Typle::JsonValue.parse(Typle::JsonValue.generate(value))

  def assert_json_equal(expected, actual, message = nil)
    assert_equal json(expected), json(actual), message
  end

  def library_hash(library) = { "packs" => library.packs, "lists" => library.lists }

  test "the limits are the ones of tests/fixtures/limits.json" do
    Fixtures.read("limits.json").each do |name, value|
      next if name == "MAX_IMPORT_BYTES"

      assert_equal value, Typle::Limits.const_get(name), name
    end
  end

  test "the default interests are those of tests/fixtures/default-interests.json" do
    assert_equal Fixtures.read("default-interests.json"), Typle::Interests::DEFAULT_INTERESTS
  end

  test "save bodies are accepted and refused as the backend does (workspace-put)" do
    Fixtures.read("workspace-put.json")["cases"].each do |item|
      name = item["name"]
      body = begin
        Typle::JsonValue.parse(item["body"])
      rescue JSON::ParserError
        nil
      end
      unless body.is_a?(Hash)
        refute item["accepted"], name
        next
      end
      change = begin
        Typle::WorkspaceRequest.validate(body)
      rescue StandardError
        nil
      end
      assert_equal item["accepted"], !change.nil?, name
      if change
        revision, checked = change
        assert_equal item["revision"], revision, name
        assert_equal item["lists"], checked.lists&.size || -1, name
        assert_equal item["devices"], checked.devices&.size || -1, name
      end
      read = Typle::WorkspaceChanges.read(body)
      assert_equal item["accepted"], read.first == :ok, name
      assert_includes %w[invalidWorkspace levelsConflict], read[1], name if read.first == :refused
    end
  end

  LIST = '{"id":"a","name":"a","createdAt":"2026-09-06","words":[{"display":"a","input":"a","annotation":""}],"records":[]}'

  def body(fields) = Typle::JsonValue.parse(%({"revision":1,"accountId":"github:1",#{fields}}))

  test "a body is refused with invalidWorkspace for what the Go backend refused" do
    [
      %("profileId":"p","lists":[#{LIST}]), %("packs":[]), %("lists":null), %("lists":[#{LIST}],"packs":null),
      %("devices":null), %("lists":[#{LIST.sub('"name":"a"', '"name":"a\\u0000b"')}]), %("interests":["a\\u0000"]),
      %("lists":[#{LIST.sub('"words":[{"display":"a","input":"a","annotation":""}]', '"words":[]')}]), %("lists":"x"),
    ].each do |fields|
      assert_equal [:refused, "invalidWorkspace"], Typle::WorkspaceChanges.read(body(fields)), fields
    end
    # A lone surrogate does not get past the JSON parser at all (the save is refused with invalidWorkspace there).
    assert_raises(JSON::ParserError) { body(%("lists":[#{LIST.sub('"name":"a"', '"name":"a\\ud800b"')}])) }
    assert_equal :ok, Typle::WorkspaceChanges.read(body(%("lists":[#{LIST.sub('"name":"a"', '"name":"a\\ud83d\\ude00"')}]))).first
  end

  test "interests sent as null are none, and lists of several levels sent with packs are taken" do
    _, _, none = Typle::WorkspaceChanges.read(body('"interests":null'))
    assert_equal [], none.interests
    packs = '"packs":[{"id":"p","name":"P","createdAt":"2026-09-06"}]'
    several = Typle::WorkspaceChanges.read(body(%(#{packs},"lists":[#{LIST.sub('"records":[]', '"records":[],"packId":"p","levels":["初級","文法"]')}])))
    assert_equal :ok, several.first
    assert_equal %w[初級 文法], several[2].lists[0]["levels"]
    x = LIST.sub('"id":"a","name":"a"', '"id":"x","name":"x"').sub('"records":[]', '"records":[],"packId":"p"')
    y = LIST.sub('"id":"a","name":"a"', '"id":"y","name":"y"').sub('"records":[]', '"records":[],"packId":"p","levels":["x"]')
    assert_equal [:refused, "levelsConflict"], Typle::WorkspaceChanges.read(body(%(#{packs},"lists":[#{x},#{y}])))
  end

  Fixtures.files("packs").each do |name, fixture|
    test "pack fixture #{name}" do
      result = Typle::WordPacks.normalize_library(fixture["lists"], fixture["packs"])
      assert_json_equal fixture["expected"], library_hash(result)
      Typle::WordPacks.validate_library(result)
      again = Typle::WordPacks.normalize_library(result.lists, result.packs)
      assert_json_equal library_hash(result), library_hash(again)
    end
  end

  Fixtures.files("follow").each do |name, fixture|
    test "follow fixture #{name}" do
      official = fixture["official"].map { |o| Typle::OfficialPack.new(o["catalogId"], o["version"], o["pack"], o["lists"]) }
      library = fixture["library"]
      normalized = Typle::WordPacks.normalize_library(library["lists"], library["packs"])
      once = Typle::FollowedPacks.follow_official(normalized, official)
      assert_json_equal fixture["expected"], library_hash(once)
      twice = Typle::FollowedPacks.follow_official(once, official)
      assert_json_equal library_hash(once), library_hash(twice)
      Typle::WordPacks.validate_library(once)
    end
  end

  def error_key(name) = name == "listNotFound" ? "recordListDeleted" : name

  Fixtures.files("records").each do |name, fixture|
    test "add-record fixture #{name}" do
      lists = fixture["lists"]
      run = -> { Typle::Records.add_record(lists, fixture["devices"], fixture["listId"], fixture["input"]) }
      expected = fixture["expected"]
      if expected["error"]
        assert_equal error_key(expected["error"]), assert_raises(Typle::AppError) { run.() }.key
        next
      end
      result = run.()
      assert_equal expected["added"], result.added
      assert_json_equal expected["record"], result.record
      saved = result.lists.find { |list| list["id"] == fixture["listId"] }["records"]
      assert_json_equal expected["record"], saved.last
      before = lists.find { |list| list["id"] == fixture["listId"] }["records"].size
      assert_equal before + (result.added ? 1 : 0), saved.size
    end
  end

  Fixtures.files("record-operations").each do |name, fixture|
    test "record operation fixture #{name}" do
      lists = fixture["lists"]
      fixture["steps"].each_with_index do |step, i|
        expected = step["expected"]
        if step["op"] == "add"
          if expected["error"]
            error = assert_raises(Typle::AppError, "step #{i}") { Typle::Records.add_record(lists, fixture["devices"], step["listId"], step["input"]) }
            assert_equal error_key(expected["error"]), error.key, "step #{i}"
            next
          end
          result = Typle::Records.add_record(lists, fixture["devices"], step["listId"], step["input"])
          assert_equal expected["added"], result.added, "step #{i}"
          lists = result.lists
        else
          result = Typle::Records.delete_record(lists, step["listId"], step["recordId"])
          assert_equal expected["deleted"], !result.nil?, "step #{i}"
          lists = result if result
        end
      end
      fixture["expected"].each do |list_id, records|
        assert_json_equal records, lists.find { |list| list["id"] == list_id }["records"], list_id
      end
    end
  end

  test "records without an id get the ids the Go backend gave them (record-ids.json)" do
    Fixtures.read("record-ids.json")["ids"].each do |item|
      assert_equal item["id"], Typle::RecordRows.legacy_id(item["listId"], item["position"], item["date"], item["time"])
    end
  end

  test "the official packs are built from src/lib/packs, and following them again changes nothing" do
    official = Typle::Catalog.official
    assert_operator official.size, :>, 10
    official.each do |entry|
      refute_empty entry.lists, entry.catalog_id
      Typle::WordPacks.validate_library(Typle::Library.new([entry.pack], entry.lists))
    end
    first = official[0]
    pack = first.pack.except("origin").merge("id" => "mine", "origin" => { "catalogId" => first.catalog_id, "version" => 1, "followed" => true })
    lists = first.lists.map { |list| list.merge("packId" => "mine", "origin" => { "listId" => list["id"] }, "id" => "mine:#{list["id"]}") }
    once = Typle::Catalog.followed(Typle::Library.new([pack], lists))
    twice = Typle::Catalog.followed(once)
    assert_json_equal library_hash(once), library_hash(twice)
    assert_equal first.lists.size, once.lists.size
  end

  test "a save keeps the records apart as rows and reads them back into their lists" do
    lists = [{ "id" => "l1", "records" => [{ "time" => 1500, "date" => "2026-09-07T00:00:00.000Z" }, { "id" => "r", "time" => 1, "date" => "2026-09-07" }] }]
    rows = Typle::RecordRows.rows(lists)
    assert_equal ["legacy-fac8cf22c83c8cc3ef15", "r"], rows.map(&:record_id)
    assert_equal [0, 1], rows.map(&:position)
    assert_equal Typle::JsDate.parse("2026-09-07"), rows[1].at
    assert_equal '{"time":1500,"date":"2026-09-07T00:00:00.000Z","id":"legacy-fac8cf22c83c8cc3ef15"}', rows[0].data
    back = Typle::RecordRows.with_records([{ "id" => "l", "records" => [] }, { "id" => "m", "records" => [] }], { "l" => [{ "id" => "r" }] })
    assert_equal [{ "id" => "l", "records" => [{ "id" => "r" }] }, { "id" => "m", "records" => [] }], back
  end

  test "a save keeps an unknown field holding a number JSON cannot write, as null" do
    body = Typle::JsonValue.parse(%({"revision":0,"lists":[#{LIST.sub('"records":[]', '"records":[],"x":1e400')}]}))
    _, _, change = Typle::WorkspaceChanges.read(body)
    encoded = Typle::WorkspaceChanges.encode(change)
    assert_includes Typle::JsonValue.generate(encoded.library.lists), '"x":null'
  end
end
