<?php

declare(strict_types=1);

namespace App\Models;

use App\Casts\Json;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Query\Expression;

/**
 * A user's workspace (typle_workspaces, one row per owner): lists, packs, devices and interests as jsonb, and the
 * revision every save moves on. The table belongs to typle-r (db/migrations of the repository); there are no Laravel
 * migrations for it.
 */
class Workspace extends Model
{
    protected $table = 'typle_workspaces';

    protected $primaryKey = 'owner_id';

    protected $keyType = 'string';

    public $incrementing = false;

    public $timestamps = false;

    protected $guarded = [];

    protected function casts(): array
    {
        return [
            'lists' => Json::class,
            'packs' => Json::class,
            'devices' => Json::class,
            'interests' => Json::class,
        ];
    }

    /** The records of a split row, as [list_id, data] pairs in their order; null when the records are still in the lists. */
    public const RECORDS = <<<'SQL'
        CASE WHEN records_split THEN (SELECT COALESCE(jsonb_agg(jsonb_build_array(r.list_id, r.data) ORDER BY r.list_id, r.position), '[]'::jsonb) FROM typle_records r WHERE r.owner_id = typle_workspaces.owner_id)::text END
        SQL;

    /**
     * The saved lists and followed packs, counted as ownListCount and followedPackCount of src/lib/listOverview.ts count
     * them (typle-r v2.62.0 counts the saved ones with those functions, not in SQL): a pack is followed when its origin's
     * followed is the JSON true, and the followed packs are a set of ids.
     */
    private const COUNTS = <<<'SQL'
        WITH followed AS (SELECT p->>'id' AS id FROM typle_workspaces, jsonb_array_elements(COALESCE(packs, '[]'::jsonb)) AS p WHERE owner_id = ? AND p->'origin'->'followed' = 'true'::jsonb) SELECT revision, (SELECT count(*) FROM jsonb_array_elements(lists) AS l WHERE l->>'packId' IS NULL OR l->>'packId' NOT IN (SELECT id FROM followed)) AS own, (SELECT count(DISTINCT id) FROM followed) AS followed FROM typle_workspaces WHERE owner_id = ?
        SQL;

    /** The update of a save at a revision: the parts given (null keeps one), matched to the revision in the statement itself. */
    private const SAVE = <<<'SQL'
        UPDATE typle_workspaces SET lists = COALESCE(?::jsonb, lists), packs = COALESCE(?::jsonb, packs), devices = COALESCE(?::jsonb, devices), interests = COALESCE(?::jsonb, interests), records_split = records_split OR ?::jsonb IS NOT NULL, revision = revision + 1, updated_at = now() WHERE owner_id = ? AND revision = ?
        SQL;

    /**
     * The saved row of owner: lists, packs, devices, interests and revision (cast) and its split records as JSON text
     * (records), or null.
     */
    public static function readSaved(string $owner): ?self
    {
        return static::query()->where('owner_id', $owner)
            ->first(['lists', 'packs', 'devices', 'interests', 'revision', new Expression(self::RECORDS.' AS records')]);
    }

    /**
     * The revision of owner's row and its numbers of own lists and followed packs, or null.
     *
     * @return array{int, int, int}|null
     */
    public static function counts(string $owner): ?array
    {
        $row = static::query()->getConnection()->selectOne(self::COUNTS, [$owner, $owner]);

        return $row === null ? null : [(int) $row->revision, (int) $row->own, (int) $row->followed];
    }

    /** Takes the lock that keeps two first saves of owner apart until the transaction ends. */
    public static function lockFirstSave(string $owner): void
    {
        static::query()->getConnection()->select("SELECT pg_advisory_xact_lock(hashtext('typle_workspaces:' || ?))", [$owner]);
    }

    /** Saves the parts given as JSON text over owner's row at revision; the number of rows changed (0 or 1). */
    public static function saveAtRevision(string $owner, int $revision, ?string $lists, ?string $packs, ?string $devices, ?string $interests): int
    {
        return static::query()->getConnection()->update(self::SAVE, [$lists, $packs, $devices, $interests, $lists, $owner, $revision]);
    }
}
