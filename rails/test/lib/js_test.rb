require "test_helper"

class JsTest < ActiveSupport::TestCase
  test "dates are read as Date.parse reads them" do
    iso = ->(text) { Typle::JsDate.to_iso(Typle::JsDate.parse(text)) }
    assert_equal Time.utc(2026, 10, 2, 10).to_i * 1000.0, Typle::JsDate.parse("2026-10-02T10:00:00.000Z")
    assert_equal "2026-10-02T10:00:00.000Z", iso.("2026-10-02T19:00:00+09:00")
    assert_equal "2026-10-01T00:00:00.000Z", iso.("2026-10-01")
    assert_equal "2026-03-03T00:00:00.000Z", iso.("2026-02-31")
    assert_equal "2026-10-02T10:00:00.123Z", iso.("2026-10-02T10:00:00.1239Z")
    assert_equal "2026-09-07T10:00:00.000Z", iso.("Mon, 07 Sep 2026 10:00:00 GMT")
    assert_equal "2026-09-07T01:00:00.000Z", iso.("Mon Sep 07 2026 10:00:00 GMT+0900 (JST)")
    assert_equal "2026-09-07T10:00:00.000Z", iso.("2026/09/07 10:00")
    ["not a date", "2026-13-01", "2026-10-01T25:00", ""].each { |bad| assert Typle::JsDate.parse(bad).nan?, bad }
    assert_equal 2026.0, Typle::JsDate.utc_year(Typle::JsDate.parse("2026-12-31T23:59:59Z"))
    assert_equal "+010000-01-01T00:00:00.000Z", Typle::JsDate.to_iso(Typle::JsDate.parse("+010000-01-01T00:00:00Z"))
  end

  test "numbers are written as Go writes them with %g (record-ids.json floats)" do
    Fixtures.read("record-ids.json")["floats"].each do |item|
      assert_equal item["go"], Typle::Js.go_float(item["value"].to_f), item["value"].to_s
    end
  end

  test "JSON keeps JS's numbers and duplicate keys" do
    assert_equal({ "a" => 2 }, Typle::JsonValue.parse('{"a":1,"a":2}'))
    assert_equal '{"t":1500,"a":2.5}', Typle::JsonValue.generate(Typle::JsonValue.parse('{"t":1500.0,"a":2.5}'))
    assert_kind_of Integer, Typle::JsonValue.parse("95.0")
    assert_equal 0, Typle::JsonValue.parse("-0.0")
    assert_kind_of Float, Typle::JsonValue.parse("1e300")
    assert_raises(JSON::ParserError) { Typle::JsonValue.parse("[1,]") }
    assert_raises(JSON::ParserError) { Typle::JsonValue.parse("") }
  end

  test "lengths and slices count UTF-16 units" do
    assert_equal 2, Typle::Js.length("😀")
    assert_equal 3, Typle::Js.length("a😀")
    assert_equal "a", Typle::Js.slice_to("a😀", 2)
    assert_equal "a😀", Typle::Js.slice_to("a😀", 3)
    assert_equal "x", Typle::Js.trim("　x﻿\n")
    assert_equal " x", Typle::Js.trim_end(" x ")
    assert_equal 4, Typle::Js.utf8_bytesize("😀")
  end

  test "numbers are told apart as JS's Number checks do" do
    assert Typle::Js.safe_integer?(9007199254740991)
    refute Typle::Js.safe_integer?(9007199254740992)
    refute Typle::Js.safe_integer?(1.5)
    refute Typle::Js.safe_integer?("1")
    assert Typle::Js.integer?(1e300)
    assert Typle::Js.finite_number?(2.5)
    refute Typle::Js.finite_number?(true)
  end

  test "JSON is written as JSON.stringify writes it: non-finite numbers as null, numbers as Number#toString" do
    assert_equal '{"x":null,"y":[null,null]}', Typle::JsonValue.generate({ "x" => Float::INFINITY, "y" => [-Float::INFINITY, Float::NAN] })
    assert_equal '{"x":null}', Typle::JsonValue.generate(Typle::JsonValue.parse('{"x":1e400}'))
    assert_equal "100000000000000000", Typle::JsonValue.generate(Typle::JsonValue.parse("1e17"))
    assert_equal "[1.5e-7,1e+21,0.1,2.5]", Typle::JsonValue.generate([1.5e-7, 1e21, 0.1, 2.5])
  end
end
