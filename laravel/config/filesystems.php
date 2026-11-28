<?php

// No files are served or stored: the local disk does not serve storage/ under /storage (Laravel's default does), so
// every path but the three of routes/api.php is 404 notFound.
return [
    'default' => 'local',

    'disks' => [
        'local' => [
            'driver' => 'local',
            'root' => storage_path('app'),
            'serve' => false,
            'throw' => false,
            'report' => false,
        ],
    ],
];
