<?php

declare(strict_types=1);

namespace App\Typle;

use RuntimeException;

/**
 * A plain Error of the TypeScript code (new Error(...)): not an AppError, so its caller answers it with its default.
 */
final class PlainError extends RuntimeException {}
