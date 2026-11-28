<?php

declare(strict_types=1);

namespace App\Http\Middleware;

use App\Http\Answer;
use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

/**
 * The comparison's gate and the headers of every answer (rails/app/middleware/typle/backend_gate.rb): outside the
 * comparison (TYPLE_BACKEND_COMPARE is not 1) every request is 404, so a deployment without it reads and writes nothing.
 * Every answer carries the headers next.config.mjs adds (and its CSP of /api) and x-typle-backend: laravel. It is the
 * only global middleware (bootstrap/app.php): no session, cookies, CSRF, CORS or input trimming.
 */
class BackendGate
{
    public const BACKEND_HEADER = 'x-typle-backend';

    public const SECURITY_HEADERS = [
        'x-frame-options' => 'DENY',
        'x-content-type-options' => 'nosniff',
        'referrer-policy' => 'strict-origin-when-cross-origin',
        'permissions-policy' => 'camera=(), microphone=(), geolocation=()',
        'content-security-policy' => "frame-ancestors 'none'; base-uri 'self'; object-src 'none'",
    ];

    public function handle(Request $request, Closure $next): Response
    {
        self::useRealMethod($request);
        $response = config('typle.compare') ? $next($request) : Answer::error(404, 'notFound');
        $response->headers->add(self::SECURITY_HEADERS);
        $response->headers->set(self::BACKEND_HEADER, 'laravel');

        // A HEAD answer has no body, whatever answered it.
        return $response->prepare($request);
    }

    /**
     * The request's method is the one it was sent with: Symfony would take a POST as the method of an
     * X-HTTP-Method-Override header or of a _method field (of the query, a form or a JSON body), which Next.js does not.
     */
    private static function useRealMethod(Request $request): void
    {
        if ($request->getRealMethod() !== 'POST') {
            return;
        }
        $request->headers->remove('X-HTTP-Method-Override');
        $request->query->remove('_method');
        $request->request->remove('_method');
        $request->setMethod('POST');
    }
}
