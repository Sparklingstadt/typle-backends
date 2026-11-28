<?php

use App\Http\Answer;
use App\Http\Controllers\ErrorsController;
use App\Http\Middleware\BackendGate;
use App\Typle\Report;
use Illuminate\Foundation\Application;
use Illuminate\Foundation\Configuration\Exceptions;
use Illuminate\Foundation\Configuration\Middleware;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Route;
use Illuminate\Support\Str;
use Symfony\Component\HttpKernel\Exception\HttpExceptionInterface;
use Symfony\Component\HttpKernel\Exception\MethodNotAllowedHttpException;

// The Laravel backend of the comparison of the backends (docs/backend-compare.md): /api/health, /api/workspace and
// /api/workspace/records answered as the Next.js Route Handlers answer them, and 404 notFound for every other path.
return Application::configure(basePath: dirname(__DIR__))
    ->withRouting(
        api: __DIR__.'/../routes/api.php',
        then: fn () => Route::fallback([ErrorsController::class, 'notFound']),
    )
    ->withMiddleware(function (Middleware $middleware): void {
        // Only the gate: no CORS, trusted proxies, maintenance mode, post size, input trimming, sessions or CSRF (none of
        // them is in the Next.js handlers), and the api group adds nothing either.
        $middleware->use([BackendGate::class]);
        $middleware->group('api', []);
    })
    ->withExceptions(function (Exceptions $exceptions): void {
        // Failures are logged once, by the answer below (App\Typle\Report), as the handlers log them.
        $exceptions->dontReport([Throwable::class]);

        $exceptions->render(function (Throwable $error, Request $request) {
            if ($error instanceof MethodNotAllowedHttpException && isset(ErrorsController::ALLOW[$request->path()])) {
                return ErrorsController::notAllowed(ErrorsController::ALLOW[$request->path()]);
            }
            if ($error instanceof HttpExceptionInterface) {
                return Answer::error(404, 'notFound');
            }

            // A failure nothing else answered: a 503 as the handlers answer one, in the log.
            $route = $request->route();
            $operation = $route === null ? 'request' : Str::snake(Str::beforeLast(class_basename($route->getControllerClass() ?? ''), 'Controller')).'.'.$route->getActionMethod();
            app(Report::class)->failure($request, $error, $operation);

            return Answer::error(503, $request->isMethod('GET') || $request->isMethod('HEAD') ? 'databaseUnavailable' : 'workspaceSaveFailed');
        });
    })->create();
