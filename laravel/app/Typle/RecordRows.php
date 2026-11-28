<?php

declare(strict_types=1);

namespace App\Typle;

use stdClass;
use TypeError;

/**
 * The rows of typle_records (src/server/records.ts): one per practice record, put back in their lists on reads.
 */
final class RecordRows
{
    /** The id a record without one (or with an earlier record's id) is given: the same record always gets the same id. */
    public static function legacyId(string $listId, int $position, string $date, int|float $time): string
    {
        return 'legacy-'.substr(hash('sha256', "{$listId}\0{$position}\0{$date}\0".Js::goFloat($time)), 0, 20);
    }

    /**
     * The records of the lists as rows, every record given an id that keeps it apart from the rest of its list, in place
     * (as recordRows does, so the size of a save counts the ids too).
     *
     * @return list<RecordRow>
     */
    public static function rows(array $lists): array
    {
        $rows = [];
        foreach ($lists as $list) {
            $seen = [];
            $listId = $list->id;
            foreach ($list->records as $position => $record) {
                $id = Value::prop($record, 'id');
                if (! is_string($id) || $id === '' || isset($seen[$id])) {
                    $record->id = self::legacyId($listId, $position, $record->date, $record->time);
                }
                $seen[$record->id] = true;
                $rows[] = new RecordRow($listId, $record->id, $position, JsDate::parse($record->date), JsonValue::generate($record));
            }
        }

        return $rows;
    }

    /**
     * The lists with the stored records put back in the lists that hold them (withRecords).
     *
     * @param  array<string, list<mixed>>  $stored
     */
    public static function withRecords(array $lists, array $stored): array
    {
        return array_map(static function ($item) use ($stored) {
            if ($item === null) {
                throw new TypeError("Cannot read properties of null (reading 'id')");
            }
            $id = Value::prop($item, 'id');
            $moved = is_string($id) ? ($stored[$id] ?? []) : [];
            if ($moved === []) {
                return $item;
            }
            $movedIds = [];
            foreach ($moved as $record) {
                $movedIds[self::recordIdOf($record)] = true;
            }
            $own = Value::prop($item, 'records');
            if (! is_array($own)) {
                $own = [];
            }
            $kept = array_values(array_filter($own, static fn ($record) => ($rid = self::recordIdOf($record)) === '' || ! isset($movedIds[$rid])));

            return Value::merge($item instanceof stdClass ? $item : new stdClass, ['records' => [...$kept, ...$moved]]);
        }, $lists);
    }

    private static function recordIdOf(mixed $record): string
    {
        $id = Value::prop($record, 'id');

        return is_string($id) ? $id : '';
    }
}
