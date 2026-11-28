<?php

namespace Tests\Lib;

use App\Typle\Js;
use App\Typle\JsDate;
use App\Typle\JsonValue;
use JsonException;
use Tests\Fixtures;
use Tests\TestCase;

class JsTest extends TestCase
{
    public function test_dates_are_read_as_date_parse_reads_them(): void
    {
        $iso = static fn (string $text): string => JsDate::toIso(JsDate::parse($text));
        $this->assertSame(gmmktime(10, 0, 0, 10, 2, 2026) * 1000.0, JsDate::parse('2026-10-02T10:00:00.000Z'));
        $this->assertSame('2026-10-02T10:00:00.000Z', $iso('2026-10-02T19:00:00+09:00'));
        $this->assertSame('2026-10-01T00:00:00.000Z', $iso('2026-10-01'));
        $this->assertSame('2026-03-03T00:00:00.000Z', $iso('2026-02-31'));
        $this->assertSame('2026-10-02T10:00:00.123Z', $iso('2026-10-02T10:00:00.1239Z'));
        $this->assertSame('2026-09-07T10:00:00.000Z', $iso('Mon, 07 Sep 2026 10:00:00 GMT'));
        $this->assertSame('2026-09-07T01:00:00.000Z', $iso('Mon Sep 07 2026 10:00:00 GMT+0900 (JST)'));
        $this->assertSame('2026-09-07T10:00:00.000Z', $iso('2026/09/07 10:00'));
        foreach (['not a date', '2026-13-01', '2026-10-01T25:00', ''] as $bad) {
            $this->assertNan(JsDate::parse($bad), $bad);
        }
        $this->assertSame(2026.0, JsDate::utcYear(JsDate::parse('2026-12-31T23:59:59Z')));
        $this->assertSame('+010000-01-01T00:00:00.000Z', JsDate::toIso(JsDate::parse('+010000-01-01T00:00:00Z')));
    }

    public function test_numbers_are_written_as_go_writes_them_with_g(): void
    {
        foreach (Fixtures::read('record-ids.json')->floats as $item) {
            $this->assertSame($item->go, Js::goFloat((float) $item->value), (string) $item->value);
        }
    }

    public function test_json_keeps_js_numbers_and_duplicate_keys(): void
    {
        $this->assertEquals((object) ['a' => 2], JsonValue::parse('{"a":1,"a":2}'));
        $this->assertSame('{"t":1500,"a":2.5}', JsonValue::generate(JsonValue::parse('{"t":1500.0,"a":2.5}')));
        $this->assertIsInt(JsonValue::parse('95.0'));
        $this->assertSame(0, JsonValue::parse('-0.0'));
        $this->assertIsFloat(JsonValue::parse('1e300'));
        foreach (['[1,]', ''] as $bad) {
            try {
                JsonValue::parse($bad);
                $this->fail("parsed {$bad}");
            } catch (JsonException) {
                $this->addToAssertionCount(1);
            }
        }
    }

    public function test_lengths_and_slices_count_utf16_units(): void
    {
        $this->assertSame(2, Js::length('😀'));
        $this->assertSame(3, Js::length('a😀'));
        $this->assertSame('a', Js::sliceTo('a😀', 2));
        $this->assertSame('a😀', Js::sliceTo('a😀', 3));
        $this->assertSame('x', Js::trim("　x\u{FEFF}\n"));
        $this->assertSame(' x', Js::trimEnd(' x '));
        $this->assertSame(4, Js::utf8Bytesize('😀'));
    }

    public function test_numbers_are_told_apart_as_js_number_checks_do(): void
    {
        $this->assertTrue(Js::safeInteger(9007199254740991));
        $this->assertFalse(Js::safeInteger(9007199254740992));
        $this->assertFalse(Js::safeInteger(1.5));
        $this->assertFalse(Js::safeInteger('1'));
        $this->assertTrue(Js::integer(1e300));
        $this->assertTrue(Js::finiteNumber(2.5));
        $this->assertFalse(Js::finiteNumber(true));
    }

    public function test_json_is_written_as_json_stringify_writes_it(): void
    {
        $this->assertSame('{"x":null,"y":[null,null]}', JsonValue::generate(['x' => INF, 'y' => [-INF, NAN]]));
        $this->assertSame('{"x":null}', JsonValue::generate(JsonValue::parse('{"x":1e400}')));
        $this->assertSame('100000000000000000', JsonValue::generate(JsonValue::parse('1e17')));
        $this->assertSame('[1.5e-7,1e+21,0.1,2.5]', JsonValue::generate([1.5e-7, 1e21, 0.1, 2.5]));
    }

    // Beyond the Rails tests: what PHP's JSON does otherwise than Ruby's or JavaScript's.

    public function test_objects_stay_objects_whatever_their_keys(): void
    {
        $this->assertSame('{"0":"a","":{},"b":[]}', JsonValue::generate(JsonValue::parse('{"0":"a","":{},"b":[]}')));
        $this->assertSame('[0.00001,0.000001,1e-7,-0.5,123456789012345680000,0]', JsonValue::generate([1e-5, 1e-6, 1e-7, -0.5, 1.2345678901234568e20, -0.0]));
        $this->assertSame('{"s":"\u001f\b\f/é "}', str_replace("\u{2028}", ' ', JsonValue::generate(['s' => "\x1f\x08\x0c/é\u{2028}"])));
        $this->assertSame('9007199254740992', JsonValue::generate(JsonValue::parse('9007199254740993')));
    }

    public function test_json_nests_as_deep_as_ruby_allows(): void
    {
        $this->assertIsArray(JsonValue::parse(str_repeat('[', 100).str_repeat(']', 100)));
        $this->expectException(JsonException::class);
        JsonValue::parse(str_repeat('[', 101).str_repeat(']', 101));
    }
}
