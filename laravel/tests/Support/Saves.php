<?php

namespace Tests\Support;

use App\Typle\AppError;
use App\Typle\Conflict;
use App\Typle\Value;
use App\Typle\WorkspaceChanges;
use App\Typle\WorkspaceStore;
use stdClass;

/**
 * A save of a body as /api/workspace answers it: ok, or the key it is refused with (the save of
 * rails/test/integration/workspace_test.rb).
 */
final class Saves
{
    public static function save(string $owner, stdClass $body): string
    {
        $read = WorkspaceChanges::read(Value::merge($body, ['accountId' => $owner]));
        if ($read[0] === 'refused') {
            return $read[1];
        }

        try {
            app(WorkspaceStore::class)->write($owner, $read[1], $read[2]);

            return 'ok';
        } catch (Conflict) {
            return 'conflict';
        } catch (AppError $error) {
            return $error->key;
        }
    }
}
