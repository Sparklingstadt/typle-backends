<?php

declare(strict_types=1);

namespace App\Http\Middleware;

use App\Http\Answer;
use App\Typle\Js;
use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

/**
 * A write comes from the app's own pages (its Origin) and declares a JSON body (src/server/request.ts), else 403
 * forbiddenRequest. Headers are read as Node reads them (one byte one character). An Authorization header stands in
 * for neither.
 */
class RequireWriteRequest
{
    public function handle(Request $request, Closure $next): Response
    {
        $settings = $request->attributes->get(RequireLoginSettings::ATTRIBUTE);
        $contentType = Js::trim(explode(';', (string) Js::header($request->headers->get('Content-Type')))[0]);
        $origin = Js::header($request->headers->get('Origin'));
        if ($origin !== $settings->origin || $contentType !== 'application/json') {
            return Answer::error(403, 'forbiddenRequest');
        }

        return $next($request);
    }
}
