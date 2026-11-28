<?php

declare(strict_types=1);

namespace App\Models;

use App\Casts\Json;
use App\Typle\PgArray;
use App\Typle\RecordRow;
use Illuminate\Database\Eloquent\Model;

/**
 * A practice record (typle_records, one row per record of a list, keyed by owner_id, list_id and record_id). at is
 * written as the text PostgreSQL reads (App\Typle\WorkspaceStore::timestamp), and data as the JSON the record was
 * written to once. The table belongs to typle-r.
 */
class PracticeRecord extends Model
{
    protected $table = 'typle_records';

    protected $primaryKey = null;

    public $incrementing = false;

    public $timestamps = false;

    protected $guarded = [];

    protected function casts(): array
    {
        return ['data' => Json::class];
    }

    /** The rows of owner, but a row is changed only where it differs (src/server/records.ts). */
    private const WRITE = <<<'SQL'
        INSERT INTO typle_records (owner_id, list_id, record_id, position, at, data) SELECT ?, k.list_id, k.record_id, k.position, k.at, k.data FROM unnest(?::text[], ?::text[], ?::int[], ?::timestamptz[], ?::jsonb[]) AS k(list_id, record_id, position, at, data) ON CONFLICT (owner_id, list_id, record_id) DO UPDATE SET position = EXCLUDED.position, at = EXCLUDED.at, data = EXCLUDED.data WHERE (typle_records.position, typle_records.at, typle_records.data) IS DISTINCT FROM (EXCLUDED.position, EXCLUDED.at, EXCLUDED.data)
        SQL;

    /**
     * Deletes the rows of owner but those of keys ([list_id, record_id] pairs).
     *
     * @param  list<array{string, string}>  $keys
     */
    public static function deleteAllBut(string $owner, array $keys): int
    {
        return static::query()->where('owner_id', $owner)
            ->whereRaw('NOT EXISTS (SELECT 1 FROM unnest(?::text[], ?::text[]) AS k(list_id, record_id) WHERE k.list_id = typle_records.list_id AND k.record_id = typle_records.record_id)',
                [PgArray::encode(array_column($keys, 0)), PgArray::encode(array_column($keys, 1))])
            ->delete();
    }

    /**
     * Writes the rows of owner in one statement, changing a saved row only where it differs.
     *
     * @param  list<RecordRow>  $rows
     * @param  callable(float): string  $timestamp
     */
    public static function writeAll(string $owner, array $rows, callable $timestamp): void
    {
        $column = static fn (callable $value): string => PgArray::encode(array_map($value, $rows));
        static::query()->getConnection()->statement(self::WRITE, [
            $owner,
            $column(static fn (RecordRow $row) => $row->listId),
            $column(static fn (RecordRow $row) => $row->recordId),
            $column(static fn (RecordRow $row) => $row->position),
            $column(static fn (RecordRow $row) => $timestamp($row->at)),
            $column(static fn (RecordRow $row) => $row->data),
        ]);
    }
}
