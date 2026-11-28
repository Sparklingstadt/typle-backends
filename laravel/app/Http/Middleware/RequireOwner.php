<?php

declare(strict_types=1);

namespace App\Http\Middleware;

use App\Http\Answer;
use App\Typle\Report;
use App\Typle\Sessions;
use App\Typle\SessionUsers;
use Closure;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Date;
use RuntimeException;
use Symfony\Component\HttpFoundation\Response;
use Throwable;

/**
 * signedIn of src/server/account.ts: the user of the Better Auth session the login cookie names, kept on the request as
 * the owner of the data. loginRequired without a session cookie, with one whose signature does not check out, or with
 * one of no session or an expired one; databaseUnavailable when the session cannot be read (also without
 * BETTER_AUTH_SECRET, as Better Auth then fails to start), as the data cannot be either. No cookie is removed: typle-r
 * reads the session through auth.api.getSession, whose cookies do not reach the answer. The session row is always read
 * (typle-r's cookie cache may answer for up to 5 minutes without it), so a session deleted by signing out of every device
 * is refused at once. The shared demo account's sessions count only while its login is on (DEMO_LOGIN,
 * src/server/demoLogin.ts).
 */
class RequireOwner
{
    public const ATTRIBUTE = 'typle.owner';

    /** The shared demo account's ID (src/server/demoLogin.ts). */
    public const DEMO_ACCOUNT_ID = 'demo:shared';

    public function __construct(private SessionUsers $sessions, private Report $report) {}

    public function handle(Request $request, Closure $next): Response
    {
        $settings = $request->attributes->get(RequireLoginSettings::ATTRIBUTE);
        if ($settings->secret === '') {
            $this->report->failure($request, new RuntimeException('BETTER_AUTH_SECRET is not set'), 'session.check');

            return Answer::error(503, 'databaseUnavailable');
        }
        $value = Sessions::cookieValue($request->headers->get('Cookie'), $settings->cookieName());
        $token = $value === null ? null : Sessions::token($value, $settings->secret);
        if ($token === null) {
            return Answer::error(401, 'loginRequired');
        }

        try {
            $user = $this->sessions->userOf($token, Date::now());
        } catch (Throwable $error) {
            $this->report->failure($request, $error, 'session.check');

            return Answer::error(503, 'databaseUnavailable');
        }
        if ($user === null || ($user->sub === self::DEMO_ACCOUNT_ID && ! $settings->demoLogin)) {
            return Answer::error(401, 'loginRequired');
        }
        $request->attributes->set(self::ATTRIBUTE, $user->sub);

        return $next($request);
    }
}
