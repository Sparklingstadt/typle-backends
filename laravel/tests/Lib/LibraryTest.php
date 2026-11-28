<?php

namespace Tests\Lib;

use App\Typle\AppError;
use App\Typle\Catalog;
use App\Typle\FollowedPacks;
use App\Typle\Interests;
use App\Typle\JsDate;
use App\Typle\JsonValue;
use App\Typle\Library;
use App\Typle\Limits;
use App\Typle\OfficialPack;
use App\Typle\RecordRows;
use App\Typle\Records;
use App\Typle\Value;
use App\Typle\WordPacks;
use App\Typle\WorkspaceChanges;
use App\Typle\WorkspaceRequest;
use JsonException;
use PHPUnit\Framework\Attributes\DataProvider;
use stdClass;
use Tests\Fixtures;
use Tests\TestCase;
use Throwable;

/**
 * The checks and changes of src/lib, held to the shared fixtures as rails/test/lib/library_test.rb holds them.
 */
class LibraryTest extends TestCase
{
    private const LIST = '{"id":"a","name":"a","createdAt":"2026-09-06","words":[{"display":"a","input":"a","annotation":""}],"records":[]}';


    private function assertJsonEqual(mixed $expected, mixed $actual, string $message = ''): void
    {
        $this->assertSame(self::comparable($expected), self::comparable($actual), $message);
    }

    private static function libraryHash(Library $library): array
    {
        return ['packs' => $library->packs, 'lists' => $library->lists];
    }

    /** The packs of a fixture, null when it has none. */
    private static function packsOf(stdClass $value): ?array
    {
        $packs = Value::prop($value, 'packs');

        return is_array($packs) ? $packs : null;
    }

    private static function body(string $fields): stdClass
    {
        return JsonValue::parse('{"revision":1,"accountId":"github:1",'.$fields.'}');
    }

    public function test_the_limits_are_the_ones_of_limits_json(): void
    {
        foreach (Fixtures::read('limits.json') as $name => $value) {
            if ($name === 'MAX_IMPORT_BYTES') {
                continue;
            }
            $this->assertSame($value, constant(Limits::class.'::'.$name), $name);
        }
    }

    public function test_the_default_interests_are_those_of_default_interests_json(): void
    {
        $this->assertSame(Fixtures::read('default-interests.json'), Interests::defaults());
    }

    public function test_save_bodies_are_accepted_and_refused_as_the_backend_does(): void
    {
        foreach (Fixtures::read('workspace-put.json')->cases as $item) {
            $name = $item->name;
            try {
                $body = JsonValue::parse($item->body);
            } catch (JsonException) {
                $body = null;
            }
            if (! $body instanceof stdClass) {
                $this->assertFalse($item->accepted, $name);

                continue;
            }
            try {
                $change = WorkspaceRequest::validate($body);
            } catch (Throwable) {
                $change = null;
            }
            $this->assertSame($item->accepted, $change !== null, $name);
            if ($change !== null) {
                [$revision, $checked] = $change;
                $this->assertSame($item->revision, $revision, $name);
                $this->assertSame($item->lists, $checked->lists === null ? -1 : count($checked->lists), $name);
                $this->assertSame($item->devices, $checked->devices === null ? -1 : count($checked->devices), $name);
            }
            $read = WorkspaceChanges::read($body);
            $this->assertSame($item->accepted, $read[0] === 'ok', $name);
            if ($read[0] === 'refused') {
                $this->assertContains($read[1], ['invalidWorkspace', 'levelsConflict'], $name);
            }
        }
    }

    public function test_a_body_is_refused_with_invalid_workspace_for_what_the_go_backend_refused(): void
    {
        $list = self::LIST;
        foreach ([
            '"profileId":"p","lists":['.$list.']', '"packs":[]', '"lists":null', '"lists":['.$list.'],"packs":null',
            '"devices":null', '"lists":['.str_replace('"name":"a"', '"name":"a\\u0000b"', $list).']', '"interests":["a\\u0000"]',
            '"lists":['.str_replace('"words":[{"display":"a","input":"a","annotation":""}]', '"words":[]', $list).']', '"lists":"x"',
        ] as $fields) {
            $this->assertSame(['refused', 'invalidWorkspace'], WorkspaceChanges::read(self::body($fields)), $fields);
        }
        // A lone surrogate does not get past the JSON parser at all (the save is refused with invalidWorkspace there).
        try {
            self::body('"lists":['.str_replace('"name":"a"', '"name":"a\\ud800b"', $list).']');
            $this->fail('a lone surrogate was read');
        } catch (JsonException) {
            $this->addToAssertionCount(1);
        }
        $this->assertSame('ok', WorkspaceChanges::read(self::body('"lists":['.str_replace('"name":"a"', '"name":"a\\ud83d\\ude00"', $list).']'))[0]);
    }

    public function test_interests_sent_as_null_are_none_and_lists_of_several_levels_sent_with_packs_are_taken(): void
    {
        [, , $none] = WorkspaceChanges::read(self::body('"interests":null'));
        $this->assertSame([], $none->interests);
        $packs = '"packs":[{"id":"p","name":"P","createdAt":"2026-09-06"}]';
        $several = WorkspaceChanges::read(self::body($packs.',"lists":['.str_replace('"records":[]', '"records":[],"packId":"p","levels":["初級","文法"]', self::LIST).']'));
        $this->assertSame('ok', $several[0]);
        $this->assertSame(['初級', '文法'], $several[2]->lists[0]->levels);
        $x = str_replace('"records":[]', '"records":[],"packId":"p"', str_replace('"id":"a","name":"a"', '"id":"x","name":"x"', self::LIST));
        $y = str_replace('"records":[]', '"records":[],"packId":"p","levels":["x"]', str_replace('"id":"a","name":"a"', '"id":"y","name":"y"', self::LIST));
        $this->assertSame(['refused', 'levelsConflict'], WorkspaceChanges::read(self::body($packs.',"lists":['.$x.','.$y.']')));
    }

    public static function packFixtures(): array
    {
        return Fixtures::files('packs');
    }

    #[DataProvider('packFixtures')]
    public function test_pack_fixture(stdClass $fixture): void
    {
        $result = WordPacks::normalizeLibrary($fixture->lists, self::packsOf($fixture));
        $this->assertJsonEqual($fixture->expected, self::libraryHash($result));
        WordPacks::validateLibrary($result);
        $again = WordPacks::normalizeLibrary($result->lists, $result->packs);
        $this->assertJsonEqual(self::libraryHash($result), self::libraryHash($again));
    }

    public static function followFixtures(): array
    {
        return Fixtures::files('follow');
    }

    #[DataProvider('followFixtures')]
    public function test_follow_fixture(stdClass $fixture): void
    {
        $official = array_map(static fn ($o) => new OfficialPack($o->catalogId, $o->version, $o->pack, $o->lists), $fixture->official);
        $library = $fixture->library;
        $normalized = WordPacks::normalizeLibrary($library->lists, self::packsOf($library));
        $once = FollowedPacks::followOfficial($normalized, $official);
        $this->assertJsonEqual($fixture->expected, self::libraryHash($once));
        $twice = FollowedPacks::followOfficial($once, $official);
        $this->assertJsonEqual(self::libraryHash($once), self::libraryHash($twice));
        WordPacks::validateLibrary($once);
    }

    private static function errorKey(string $name): string
    {
        return $name === 'listNotFound' ? 'recordListDeleted' : $name;
    }

    public static function recordFixtures(): array
    {
        return Fixtures::files('records');
    }

    #[DataProvider('recordFixtures')]
    public function test_add_record_fixture(stdClass $fixture): void
    {
        $lists = $fixture->lists;
        $run = static fn () => Records::addRecord($lists, $fixture->devices, $fixture->listId, $fixture->input);
        $expected = $fixture->expected;
        if (Value::truthy(Value::prop($expected, 'error'))) {
            try {
                $run();
                $this->fail('added');
            } catch (AppError $error) {
                $this->assertSame(self::errorKey($expected->error), $error->key);
            }

            return;
        }
        $result = $run();
        $this->assertSame($expected->added, $result['added']);
        $this->assertJsonEqual($expected->record, $result['record']);
        $saved = Value::find($result['lists'], static fn ($list) => $list->id === $fixture->listId)->records;
        $this->assertJsonEqual($expected->record, end($saved));
        $before = count(Value::find($lists, static fn ($list) => $list->id === $fixture->listId)->records);
        $this->assertSame($before + ($result['added'] ? 1 : 0), count($saved));
    }

    public static function recordOperationFixtures(): array
    {
        return Fixtures::files('record-operations');
    }

    #[DataProvider('recordOperationFixtures')]
    public function test_record_operation_fixture(stdClass $fixture): void
    {
        $lists = $fixture->lists;
        foreach ($fixture->steps as $i => $step) {
            $expected = $step->expected;
            if ($step->op === 'add') {
                if (Value::truthy(Value::prop($expected, 'error'))) {
                    try {
                        Records::addRecord($lists, $fixture->devices, $step->listId, $step->input);
                        $this->fail("step {$i}");
                    } catch (AppError $error) {
                        $this->assertSame(self::errorKey($expected->error), $error->key, "step {$i}");
                    }

                    continue;
                }
                $result = Records::addRecord($lists, $fixture->devices, $step->listId, $step->input);
                $this->assertSame($expected->added, $result['added'], "step {$i}");
                $lists = $result['lists'];
            } else {
                $result = Records::deleteRecord($lists, $step->listId, $step->recordId);
                $this->assertSame($expected->deleted, $result !== null, "step {$i}");
                if ($result !== null) {
                    $lists = $result;
                }
            }
        }
        foreach ($fixture->expected as $listId => $records) {
            $this->assertJsonEqual($records, Value::find($lists, static fn ($list) => $list->id === (string) $listId)->records, (string) $listId);
        }
    }

    public function test_records_without_an_id_get_the_ids_the_go_backend_gave_them(): void
    {
        foreach (Fixtures::read('record-ids.json')->ids as $item) {
            $this->assertSame($item->id, RecordRows::legacyId($item->listId, $item->position, $item->date, $item->time));
        }
    }

    public function test_the_official_packs_are_built_from_src_lib_packs_and_following_them_again_changes_nothing(): void
    {
        $official = Catalog::official();
        $this->assertGreaterThan(10, count($official));
        foreach ($official as $entry) {
            $this->assertNotEmpty($entry->lists, $entry->catalogId);
            WordPacks::validateLibrary(new Library([$entry->pack], $entry->lists));
        }
        $first = $official[0];
        $pack = Value::merge(Value::except($first->pack, 'origin'), ['id' => 'mine', 'origin' => Value::object(['catalogId' => $first->catalogId, 'version' => 1, 'followed' => true])]);
        $lists = array_map(static fn ($list) => Value::merge($list, ['packId' => 'mine', 'origin' => Value::object(['listId' => $list->id]), 'id' => "mine:{$list->id}"]), $first->lists);
        $once = Catalog::followed(new Library([$pack], $lists));
        $twice = Catalog::followed($once);
        $this->assertJsonEqual(self::libraryHash($once), self::libraryHash($twice));
        $this->assertCount(count($first->lists), $once->lists);
    }

    public function test_a_save_keeps_the_records_apart_as_rows_and_reads_them_back_into_their_lists(): void
    {
        $lists = JsonValue::parse('[{"id":"l1","records":[{"time":1500,"date":"2026-09-07T00:00:00.000Z"},{"id":"r","time":1,"date":"2026-09-07"}]}]');
        $rows = RecordRows::rows($lists);
        $this->assertSame(['legacy-fac8cf22c83c8cc3ef15', 'r'], array_map(static fn ($row) => $row->recordId, $rows));
        $this->assertSame([0, 1], array_map(static fn ($row) => $row->position, $rows));
        $this->assertSame(JsDate::parse('2026-09-07'), $rows[1]->at);
        $this->assertSame('{"time":1500,"date":"2026-09-07T00:00:00.000Z","id":"legacy-fac8cf22c83c8cc3ef15"}', $rows[0]->data);
        $back = RecordRows::withRecords(JsonValue::parse('[{"id":"l","records":[]},{"id":"m","records":[]}]'), ['l' => [JsonValue::parse('{"id":"r"}')]]);
        $this->assertSame('[{"id":"l","records":[{"id":"r"}]},{"id":"m","records":[]}]', JsonValue::generate($back));
    }

    public function test_a_save_keeps_an_unknown_field_holding_a_number_json_cannot_write_as_null(): void
    {
        $body = JsonValue::parse('{"revision":0,"lists":['.str_replace('"records":[]', '"records":[],"x":1e400', self::LIST).']}');
        [, , $change] = WorkspaceChanges::read($body);
        $encoded = WorkspaceChanges::encode($change);
        $this->assertStringContainsString('"x":null', JsonValue::generate($encoded->library->lists));
    }
}
