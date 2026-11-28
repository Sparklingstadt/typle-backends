<?php

// The application itself: an API that answers JSON only (no views, sessions, encryption or locale of its own).
return [
    'name' => 'typle-laravel',
    'env' => env('APP_ENV', 'production'),
    // Failures are answered with the JSON 503s of the handlers whatever the value (bootstrap/app.php).
    'debug' => (bool) env('APP_DEBUG', false),
    'url' => env('APP_URL', 'http://localhost'),
    // node-postgres writes Dates in UTC, and the servers run in UTC.
    'timezone' => 'UTC',
    'locale' => 'en',
    'fallback_locale' => 'en',
    // Nothing is encrypted (the login cookie is Better Auth's, read as it is), so no APP_KEY is needed.
    'cipher' => 'AES-256-CBC',
    'key' => env('APP_KEY'),
    'previous_keys' => [],
    'maintenance' => [
        'driver' => 'file',
    ],
];
