<?php

declare(strict_types=1);

namespace App\Http\Controllers;

use App\Http\Answer;
use App\Typle\AppError;
use App\Typle\Records;
use App\Typle\Report;
use App\Typle\Value;
use App\Typle\WorkspaceDocument;
use App\Typle\WorkspaceStore;
use Illuminate\Http\Request;
use RuntimeException;
use stdClass;
use Throwable;

/**
 * /api/workspace/records: POST adds a practice record to a list, DELETE deletes one.
 */
class RecordsController extends Controller
{
    private const RECORD_FIELDS = ['id', 'time', 'date', 'mistakes', 'accuracy', 'wordCount', 'deviceId', 'device', 'mode', 'missed'];

    public function __construct(private WorkspaceStore $store, private Report $report) {}

    public function store(Request $request): Answer
    {
        $fields = $this->saveFields($request, 'invalidRecord');
        if ($fields instanceof Answer) {
            return $fields;
        }
        $owner = $this->owner($request);
        try {
            $add = self::readAdd($fields);
            if ($add === null) {
                return $this->refuse(400, 'invalidRecord');
            }
            [$listId, $input] = $add;

            [$result, $saved] = $this->store->modify($owner, static function (WorkspaceDocument $document) use ($listId, $input): array {
                $added = Records::addRecord($document->lists, $document->devices, $listId, $input);

                return [new WorkspaceDocument($document->packs, $added['lists'], $document->devices, $document->interests), $added['added'], $added];
            });
            if ($saved === null) {
                return $this->refusal($request, new RuntimeException('workspace missing after a record was added'), 'record.add');
            }

            $answer = WorkspaceStore::answer($saved, $owner);
            $answer->record = $result['record'];

            return $this->reply($result['added'] ? 201 : 200, $answer);
        } catch (Throwable $error) {
            return $this->refusal($request, $error, 'record.add');
        }
    }

    public function destroy(Request $request): Answer
    {
        $fields = $this->saveFields($request, 'invalidRecord');
        if ($fields instanceof Answer) {
            return $fields;
        }
        $owner = $this->owner($request);
        try {
            $delete = self::readDelete($fields);
            if ($delete === null) {
                return $this->refuse(400, 'invalidRecord');
            }
            [$listId, $recordId] = $delete;

            [, $saved] = $this->store->modify($owner, static function (WorkspaceDocument $document) use ($listId, $recordId): array {
                $lists = Records::deleteRecord($document->lists, $listId, $recordId);

                return [$lists !== null ? new WorkspaceDocument($document->packs, $lists, $document->devices, $document->interests) : $document, $lists !== null, null];
            });
            if ($saved === null) {
                return $this->refusal($request, new RuntimeException('workspace missing after a record was deleted'), 'record.delete');
            }

            return $this->reply(200, WorkspaceStore::answer($saved, $owner));
        } catch (Throwable $error) {
            return $this->refusal($request, $error, 'record.delete');
        }
    }

    /**
     * readRecordAdd: the list and record of a body that is exactly accountId, listId and record.
     *
     * @return array{string, stdClass}|null
     */
    private static function readAdd(stdClass $fields): ?array
    {
        $listId = Value::prop($fields, 'listId');
        $record = Value::prop($fields, 'record');
        if (Value::size($fields) !== 3 || ! is_string($listId) || $listId === '' || ! $record instanceof stdClass) {
            return null;
        }

        $input = clone $record;
        if (Value::any(Value::keys($input), static fn (string $key) => ! in_array($key, self::RECORD_FIELDS, true)) || ! is_string(Value::prop($input, 'id'))) {
            return null;
        }

        foreach (['wordCount', 'device', 'deviceId'] as $name) {
            if (property_exists($input, $name) && $input->{$name} === null) {
                unset($input->{$name});
            }
        }

        return [$listId, $input];
    }

    /**
     * readRecordDelete: the list and record id of a body that is exactly accountId, listId and recordId.
     *
     * @return array{string, string}|null
     */
    private static function readDelete(stdClass $fields): ?array
    {
        $listId = Value::prop($fields, 'listId');
        $recordId = Value::prop($fields, 'recordId');
        if (Value::size($fields) !== 3 || ! is_string($listId) || $listId === '' || ! is_string($recordId) || $recordId === '') {
            return null;
        }

        return [$listId, $recordId];
    }

    private function refusal(Request $request, Throwable $error, string $operation): Answer
    {
        if ($error instanceof AppError) {
            return match ($error->key) {
                'recordListDeleted' => $this->refuse(404, 'recordListDeleted'),
                'workspaceTooLarge' => $this->refuse(413, 'workspaceTooLarge'),
                default => $this->refuse(400, $error->key),
            };
        }
        $this->report->failure($request, $error, $operation);

        return $this->refuse(503, 'workspaceSaveFailed');
    }
}
