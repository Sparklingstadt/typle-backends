<?php

use Monolog\Formatter\LineFormatter;
use Monolog\Handler\NullHandler;
use Monolog\Handler\StreamHandler;

// Requests are not logged (Next.js does not log them either); failures answered with a 503 are (App\Typle\Report), on
// standard error, which Cloud Run collects.
return [
    'default' => env('LOG_CHANNEL', 'stderr'),

    'deprecations' => [
        'channel' => 'deprecations',
        'trace' => false,
    ],

    'channels' => [
        'stderr' => [
            'driver' => 'monolog',
            'level' => env('LOG_LEVEL', 'warning'),
            'handler' => StreamHandler::class,
            'formatter' => LineFormatter::class,
            'with' => [
                'stream' => 'php://stderr',
            ],
            'processors' => [],
        ],

        'null' => [
            'driver' => 'monolog',
            'handler' => NullHandler::class,
        ],

        // Deprecations of the framework and its packages are not the app's failures.
        'deprecations' => [
            'driver' => 'monolog',
            'handler' => NullHandler::class,
        ],

        'emergency' => [
            'path' => 'php://stderr',
        ],
    ],
];
