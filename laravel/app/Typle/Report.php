<?php

declare(strict_types=1);

namespace App\Typle;

use Illuminate\Database\QueryException;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Log;
use Throwable;

/**
 * src/server/report.ts: a failure answered with a 503, in the log. A service of the container, so the tests can count
 * the reports.
 */
class Report
{
    public function failure(Request $request, Throwable $error, string $operation): void
    {
        Log::error($request->getMethod().' '.$request->getPathInfo().': '.$operation.': '.$error::class.': '.self::message($error));
    }

    /**
     * The error's message; of a failed statement, the database's message and the statement without its values (Laravel
     * writes the values into its message, and a session token must not reach the log).
     */
    private static function message(Throwable $error): string
    {
        if ($error instanceof QueryException) {
            return ($error->getPrevious()?->getMessage() ?? 'query failed').' (SQL: '.$error->getSql().')';
        }

        return $error->getMessage();
    }
}
