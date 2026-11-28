<?php

declare(strict_types=1);

namespace App\Http\Middleware;

use App\Http\Answer;
use App\Typle\DatabaseUrl;
use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

/**
 * Without a database URL nothing is read or written (503 databaseUnavailable).
 */
class RequireDatabase
{
    public function handle(Request $request, Closure $next): Response
    {
        if (! DatabaseUrl::configured()) {
            return Answer::error(503, 'databaseUnavailable');
        }

        return $next($request);
    }
}
