<?php

use App\Http\Controllers\ErrorsController;
use App\Http\Controllers\HealthController;
use App\Http\Controllers\RecordsController;
use App\Http\Controllers\WorkspacesController;
use App\Http\Middleware\RequireDatabase;
use App\Http\Middleware\RequireLoginSettings;
use App\Http\Middleware\RequireOwner;
use App\Http\Middleware\RequireWriteRequest;
use Illuminate\Support\Facades\Route;

// The three paths of the comparison, under /api (bootstrap/app.php). GET routes answer HEAD too (Laravel runs them and
// drops the body). The other methods of each path are 405 with its Allow (ErrorsController; a method Laravel has no
// verb for is answered so in bootstrap/app.php).
$others = static fn (array $served): array => array_values(array_diff(['GET', 'HEAD', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS'], $served));

Route::get('health', [HealthController::class, 'show']);
Route::match($others(['GET', 'HEAD']), 'health', [ErrorsController::class, 'methodNotAllowed']);

// Before a handler, as the Next.js handlers check: the login settings, a write's Origin and Content-Type, the database,
// then the login.
Route::middleware(RequireLoginSettings::class)->group(function () use ($others) {
    Route::get('workspace', [WorkspacesController::class, 'show'])
        ->middleware([RequireDatabase::class, RequireOwner::class]);
    Route::put('workspace', [WorkspacesController::class, 'update'])
        ->middleware([RequireWriteRequest::class, RequireDatabase::class, RequireOwner::class]);

    Route::post('workspace/records', [RecordsController::class, 'store'])
        ->middleware([RequireWriteRequest::class, RequireDatabase::class, RequireOwner::class]);
    Route::delete('workspace/records', [RecordsController::class, 'destroy'])
        ->middleware([RequireWriteRequest::class, RequireDatabase::class, RequireOwner::class]);
});
Route::match($others(['GET', 'HEAD', 'PUT']), 'workspace', [ErrorsController::class, 'methodNotAllowed']);
Route::match($others(['POST', 'DELETE']), 'workspace/records', [ErrorsController::class, 'methodNotAllowed']);
