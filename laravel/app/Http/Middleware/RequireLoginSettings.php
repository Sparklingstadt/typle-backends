<?php

declare(strict_types=1);

namespace App\Http\Middleware;

use App\Http\Answer;
use App\Typle\LoginSettings;
use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

/**
 * readLoginSettings of src/server/settings.ts: without an http(s) AUTH_URL nothing that needs the login is served
 * (503 authUnavailable). The settings are kept on the request for the middleware after this one.
 */
class RequireLoginSettings
{
    public const ATTRIBUTE = 'typle.login';

    public function handle(Request $request, Closure $next): Response
    {
        $settings = LoginSettings::current();
        if ($settings === null) {
            return Answer::error(503, 'authUnavailable');
        }
        $request->attributes->set(self::ATTRIBUTE, $settings);

        return $next($request);
    }
}
