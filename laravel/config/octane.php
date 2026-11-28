<?php

use Laravel\Octane\Contracts\OperationTerminated;
use Laravel\Octane\Events\OperationReleased;
use Laravel\Octane\Events\RequestHandled;
use Laravel\Octane\Events\RequestReceived;
use Laravel\Octane\Events\RequestTerminated;
use Laravel\Octane\Events\TaskReceived;
use Laravel\Octane\Events\TaskTerminated;
use Laravel\Octane\Events\TickReceived;
use Laravel\Octane\Events\TickTerminated;
use Laravel\Octane\Events\WorkerErrorOccurred;
use Laravel\Octane\Events\WorkerStarting;
use Laravel\Octane\Events\WorkerStopping;
use Laravel\Octane\Listeners\CloseMonologHandlers;
use Laravel\Octane\Listeners\EnsureUploadedFilesAreValid;
use Laravel\Octane\Listeners\EnsureUploadedFilesCanBeMoved;
use Laravel\Octane\Listeners\FlushOnce;
use Laravel\Octane\Listeners\FlushTemporaryContainerInstances;
use Laravel\Octane\Listeners\ReportException;
use Laravel\Octane\Listeners\StopWorkerIfNecessary;
use Laravel\Octane\Octane;

// Octane on FrankenPHP (the Dockerfile): each worker boots the app once and keeps it, and its database connection,
// between requests, as the Node, Go, Kotlin and Rails servers keep theirs.
return [
    'server' => env('OCTANE_SERVER', 'frankenphp'),

    'https' => false,

    'listeners' => [
        WorkerStarting::class => [
            EnsureUploadedFilesAreValid::class,
            EnsureUploadedFilesCanBeMoved::class,
        ],

        RequestReceived::class => [
            ...Octane::prepareApplicationForNextOperation(),
            ...Octane::prepareApplicationForNextRequest(),
        ],

        RequestHandled::class => [],

        RequestTerminated::class => [],

        TaskReceived::class => [
            ...Octane::prepareApplicationForNextOperation(),
        ],

        TaskTerminated::class => [],

        TickReceived::class => [
            ...Octane::prepareApplicationForNextOperation(),
        ],

        TickTerminated::class => [],

        // The connection stays open between requests (no DisconnectFromDatabases), as the pools of the other servers do.
        OperationTerminated::class => [
            FlushOnce::class,
            FlushTemporaryContainerInstances::class,
        ],

        OperationReleased::class => [],

        WorkerErrorOccurred::class => [
            ReportException::class,
            StopWorkerIfNecessary::class,
        ],

        WorkerStopping::class => [
            CloseMonologHandlers::class,
        ],
    ],

    // The services the handlers use, resolved when a worker starts (Octane's default list also has the encrypter, cookies
    // and sessions, which this app has no use for and no APP_KEY for).
    'warm' => ['config', 'db', 'db.factory', 'db.transactions', 'log', 'router', 'routes', 'url'],

    'flush' => [],

    'tables' => [],

    'cache' => [
        'rows' => 1000,
        'bytes' => 10000,
    ],

    'watch' => [],

    'garbage' => 50,

    'max_execution_time' => 30,

    'state_file' => env('OCTANE_STATE_FILE', storage_path('logs/octane-server-state.json')),
];
