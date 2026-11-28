<?php

// One save in a process of its own (the test of two saves at once): php tests/Support/save.php <owner> <body JSON>.
use App\Typle\JsonValue;
use Illuminate\Contracts\Console\Kernel;
use Tests\Support\Saves;

require __DIR__.'/../../vendor/autoload.php';

$app = require __DIR__.'/../../bootstrap/app.php';
$app->make(Kernel::class)->bootstrap();

echo Saves::save($argv[1], JsonValue::parse($argv[2]));
