<?php

declare(strict_types=1);

namespace App\Typle;

use RuntimeException;

/**
 * An error named by a key of the app's `errors` messages (src/lib/appError.ts).
 */
final class AppError extends RuntimeException
{
    public function __construct(public readonly string $key)
    {
        parent::__construct($key);
    }
}
