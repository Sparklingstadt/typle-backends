<?php

use App\Typle\DatabaseUrl;
use Illuminate\Support\Env;

// The settings of the comparison, read as the other backends read them. The values are taken as they are in the
// environment (Env's repository, not env(), which would turn "true" or "null" into other values): BETTER_AUTH_SECRET
// may be any text, and TYPLE_BACKEND_COMPARE and DEMO_LOGIN count only when they are exactly 1.
$raw = static fn (string $name): ?string => Env::getRepository()->get($name);

return [
    // Outside the comparison every request is 404 (App\Http\Middleware\BackendGate).
    'compare' => $raw('TYPLE_BACKEND_COMPARE') === '1',

    // The login settings (App\Typle\LoginSettings): AUTH_URL as readLoginSettings of src/server/settings.ts reads it,
    // BETTER_AUTH_SECRET as betterAuthSettings of src/server/authUser.ts (trimmed there), DEMO_LOGIN as demoConfigured.
    'auth_url' => $raw('AUTH_URL'),
    'better_auth_secret' => $raw('BETTER_AUTH_SECRET'),
    'demo_login' => $raw('DEMO_LOGIN'),

    // The database's URL: DATABASE_URL, else POSTGRES_URL (src/server/database.ts), and TYPLE_TEST_DATABASE_URL in the
    // tests. null when none is set (503 databaseUnavailable).
    'database_url' => DatabaseUrl::fromEnv($raw, (string) $raw('APP_ENV')),

    // Where src/lib/packs and src/messages/ja/format.json are read from: the repository root (the parent of laravel/), or
    // TYPLE_APP_DATA (the Docker image copies them to /app-data).
    'app_data' => $raw('TYPLE_APP_DATA'),
];
