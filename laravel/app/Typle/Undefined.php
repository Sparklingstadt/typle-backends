<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * JavaScript's undefined: what Value::prop gives for a key that is not there (null is JSON's null).
 */
enum Undefined
{
    case Value;
}
