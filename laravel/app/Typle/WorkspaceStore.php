<?php

declare(strict_types=1);

namespace App\Typle;

use App\Models\PracticeRecord;
use App\Models\Workspace;
use Illuminate\Database\Query\Expression;
use Illuminate\Support\Facades\DB;
use RuntimeException;
use stdClass;

/**
 * src/server/workspace.ts and records.ts: reading, saving and changing a workspace in typle_workspaces, with its records
 * in typle_records. The statements are those of the Next.js handlers, one for one (rails/app/lib/typle/workspace_store.rb).
 * A service of the container, so the tests can stand in for it.
 */
class WorkspaceStore
{
    /** The owner's saved workspace, or null; checked as the app reads it, answered as saved. */
    public function read(string $owner): ?Saved
    {
        $row = Workspace::readSaved($owner);
        if ($row === null) {
            return null;
        }
        $lists = $row->lists;
        if ($row->records !== null) {
            $stored = [];
            foreach (JsonValue::parse($row->records) as [$listId, $record]) {
                $stored[$listId][] = $record;
            }
            $lists = RecordRows::withRecords($lists, $stored);
        }
        $saved = new Saved($lists, $row->packs, $row->devices, $row->interests, $row->revision);
        // Throws on saved data the app could not read either.
        $document = WorkspaceDocuments::decode($lists, $row->packs, $row->devices, $row->interests ?? Undefined::Value);
        Catalog::followed($document->library());

        return $saved;
    }

    /** The answer of GET and PUT /api/workspace; revision 0 and no lists when nothing is saved. */
    public static function answer(?Saved $saved, string $owner): stdClass
    {
        return Value::object([
            'lists' => self::orDefault($saved?->lists, []), 'packs' => $saved?->packs, 'devices' => self::orDefault($saved?->devices, []),
            'interests' => self::orDefault($saved?->interests, Interests::defaults()), 'revision' => $saved?->revision ?? 0,
            'accountId' => $owner,
        ]);
    }

    /** value || fallback, for JSON values (only null and false are replaced: no saved part is undefined). */
    private static function orDefault(mixed $value, mixed $fallback): mixed
    {
        return $value === null || $value === false ? $fallback : $value;
    }

    private static function json(mixed $value): ?string
    {
        return $value === null ? null : JsonValue::generate($value);
    }

    /** writeWorkspace: saves change over the owner's workspace at revision (0 creates the row), else Conflict. */
    public function write(string $owner, int $revision, WorkspaceChange $change): ?Saved
    {
        $encoded = WorkspaceChanges::encode($change);
        $library = $encoded->library;

        return DB::transaction(function () use ($owner, $revision, $encoded, $library): ?Saved {
            if ($revision === 0) {
                if ($library !== null && $library->own > Limits::MAX_LISTS) {
                    throw new AppError('listLimit');
                }
                if ($library !== null && $library->followed > Limits::MAX_FOLLOWED_PACKS) {
                    throw new AppError('followLimit');
                }

                // Two first saves of the same owner wait here for each other.
                Workspace::lockFirstSave($owner);
                $inserted = Workspace::query()->insertOrIgnoreReturning([
                    'owner_id' => $owner, 'lists' => self::json($library?->lists ?? []), 'packs' => self::json($library?->packs),
                    'devices' => self::json($encoded->devices ?? []), 'interests' => self::json($encoded->interests),
                    'records_split' => new Expression('true'),
                ], ['revision'], 'owner_id');
                if ($inserted->isEmpty()) {
                    throw new Conflict;
                }

                PracticeRecord::query()->where('owner_id', $owner)->delete();
            } else {
                [$savedRevision, $own, $followed] = Workspace::counts($owner) ?? [null, 0, 0];
                if ($savedRevision !== $revision) {
                    throw new Conflict;
                }
                if ($library !== null && $library->own > Limits::MAX_LISTS && $library->own > $own) {
                    throw new AppError('listLimit');
                }
                if ($library !== null && $library->followed > Limits::MAX_FOLLOWED_PACKS && $library->followed > $followed) {
                    throw new AppError('followLimit');
                }

                // Matching the revision in the UPDATE itself makes the comparison and the write one atomic operation.
                $updated = Workspace::saveAtRevision($owner, $revision, self::json($library?->lists), self::json($library?->packs),
                    self::json($encoded->devices), self::json($encoded->interests));
                if ($updated === 0) {
                    throw new Conflict;
                }
            }
            if ($library !== null) {
                $this->syncRecords($owner, $library->rows);
            }

            return $this->read($owner);
        });
    }

    /**
     * modifyWorkspace: changes the owner's workspace in one transaction that holds the row. change takes the document
     * and returns [next document, whether it changed, result]; returns [result, the saved workspace].
     *
     * @param  callable(WorkspaceDocument): array{WorkspaceDocument, bool, mixed}  $change
     * @return array{mixed, ?Saved}
     */
    public function modify(string $owner, callable $change): array
    {
        return DB::transaction(function () use ($owner, $change): array {
            $row = Workspace::query()->where('owner_id', $owner)->lockForUpdate()->first(['lists', 'packs', 'devices', 'records_split']);
            $document = new WorkspaceDocument([], [], [], []);
            if ($row !== null) {
                $lists = $row->lists;
                if ($row->records_split) {
                    $stored = [];
                    $records = PracticeRecord::query()->where('owner_id', $owner)->orderBy('list_id')->orderBy('position')
                        ->toBase()->get(['list_id', 'data']);
                    foreach ($records as $record) {
                        $stored[$record->list_id][] = JsonValue::parse($record->data);
                    }
                    $lists = RecordRows::withRecords($lists, $stored);
                }
                $decoded = WorkspaceDocuments::decode($lists, $row->packs, $row->devices, Undefined::Value);
                $followed = Catalog::followed($decoded->library());
                $document = new WorkspaceDocument($followed->packs, $followed->lists, $decoded->devices, $decoded->interests);
            }
            $before = WorkspaceChanges::encode(new WorkspaceChange($document->lists, $document->packs))->library->size;
            [$changedDocument, $changed, $result] = $change($document);
            if ($changed) {
                if ($row === null) {
                    throw new RuntimeException("no workspace of {$owner} to change");
                }

                $library = WorkspaceChanges::encode(new WorkspaceChange($changedDocument->lists, $changedDocument->packs, $changedDocument->devices))->library;
                if ($library->size > WorkspaceChanges::MAX_LISTS_BYTES && $library->size > $before) {
                    throw new AppError('workspaceTooLarge');
                }

                Workspace::query()->where('owner_id', $owner)->update([
                    'lists' => self::json($library->lists), 'packs' => self::json($library->packs), 'devices' => self::json($changedDocument->devices),
                    'records_split' => new Expression('true'), 'revision' => new Expression('revision + 1'), 'updated_at' => new Expression('now()'),
                ]);
                $this->syncRecords($owner, $library->rows);
            }

            return [$result, $this->read($owner)];
        });
    }

    /**
     * Makes the owner's rows of typle_records exactly rows (syncRecords): the rows of records no longer saved are deleted,
     * the others written where they changed, and the number of rows checked afterwards.
     *
     * @param  list<RecordRow>  $rows
     */
    private function syncRecords(string $owner, array $rows): void
    {
        PracticeRecord::deleteAllBut($owner, array_map(static fn (RecordRow $row) => [$row->listId, $row->recordId], $rows));
        if ($rows !== []) {
            PracticeRecord::writeAll($owner, $rows, self::timestamp(...));
        }
        $counted = PracticeRecord::query()->where('owner_id', $owner)->count();
        if ($counted !== count($rows)) {
            throw new RuntimeException("records of {$owner}: {$counted} rows saved for ".count($rows).' records');
        }
    }

    /** A timestamptz PostgreSQL reads, of a time in milliseconds (node-postgres writes a Date so, in UTC here). */
    public static function timestamp(float $time): string
    {
        $iso = JsDate::toIso($time);
        $year = (int) JsDate::utcYear($time);
        if ($year >= 1 && $year <= 9999) {
            return $iso;
        }

        $rest = substr($iso, strpos($iso, '-', 1));

        return $year > 9999 ? "{$year}{$rest}" : str_pad((string) (1 - $year), 4, '0', STR_PAD_LEFT).substr($rest, 0, -1).'Z BC';
    }
}
