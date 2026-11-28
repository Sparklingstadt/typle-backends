<?php

declare(strict_types=1);

namespace App\Http\Controllers;

use App\Http\Answer;
use App\Typle\DatabaseUrl;
use App\Typle\Report;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\DB;
use RuntimeException;
use Throwable;

/**
 * src/server/health.ts: 200 once the database answers within 5 seconds, 503 databaseUnavailable otherwise.
 */
class HealthController extends Controller
{
    public function show(Request $request, Report $report): Answer
    {
        try {
            if (! DatabaseUrl::configured()) {
                throw new RuntimeException('DATABASE_URL is not set');
            }
            DB::select('SELECT 1');

            return $this->reply(200, ['status' => 'ok']);
        } catch (Throwable $error) {
            $report->failure($request, $error, 'health');

            return $this->refuse(503, 'databaseUnavailable');
        }
    }
}
