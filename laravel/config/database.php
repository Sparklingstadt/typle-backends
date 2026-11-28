<?php

use App\Typle\DatabaseUrl;
use Illuminate\Support\Env;

// One connection: the PostgreSQL of typle-r, whose tables are used as they are (no migrations here). The URL is that of
// config/typle.php (DATABASE_URL, else POSTGRES_URL; TYPLE_TEST_DATABASE_URL in the tests), checked as node-postgres 8
// checks it, and the connection is set up as src/server/database.ts sets its pool up (App\Typle\DatabaseUrl).
$raw = static fn (string $name): ?string => Env::getRepository()->get($name);

return [
    'default' => 'pgsql',

    'connections' => [
        'pgsql' => DatabaseUrl::config(DatabaseUrl::fromEnv($raw, (string) $raw('APP_ENV'))),
    ],

    'migrations' => [
        'table' => 'migrations',
        'update_date_on_publish' => false,
    ],
];
