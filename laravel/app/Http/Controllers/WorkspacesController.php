<?php

declare(strict_types=1);

namespace App\Http\Controllers;

use App\Http\Answer;
use App\Typle\AppError;
use App\Typle\Conflict;
use App\Typle\Report;
use App\Typle\WorkspaceChanges;
use App\Typle\WorkspaceStore;
use Illuminate\Http\Request;
use Throwable;

/**
 * /api/workspace: GET reads the owner's workspace, PUT saves a change of it by revision.
 */
class WorkspacesController extends Controller
{
    public function __construct(private WorkspaceStore $store, private Report $report) {}

    public function show(Request $request): Answer
    {
        $owner = $this->owner($request);
        try {
            $saved = $this->store->read($owner);
        } catch (Throwable $error) {
            $this->report->failure($request, $error, 'workspace.read');

            return $this->refuse(503, 'databaseUnavailable');
        }

        return $this->reply(200, WorkspaceStore::answer($saved, $owner));
    }

    public function update(Request $request): Answer
    {
        $fields = $this->saveFields($request, 'invalidWorkspace');
        if ($fields instanceof Answer) {
            return $fields;
        }
        $owner = $this->owner($request);
        try {
            $read = WorkspaceChanges::read($fields);
            if ($read[0] === 'refused') {
                return $this->refuse(400, $read[1]);
            }
            [, $revision, $change] = $read;

            return $this->reply(200, WorkspaceStore::answer($this->store->write($owner, $revision, $change), $owner));
        } catch (Conflict) {
            return $this->refuse(409, 'conflict');
        } catch (AppError $error) {
            if (! in_array($error->key, ['listLimit', 'followLimit', 'invalidWorkspace'], true)) {
                throw $error;
            }

            return $this->refuse(400, $error->key);
        } catch (Throwable $error) {
            $this->report->failure($request, $error, 'workspace.write');

            return $this->refuse(503, 'workspaceSaveFailed');
        }
    }
}
