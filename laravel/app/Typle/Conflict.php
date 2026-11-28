<?php

declare(strict_types=1);

namespace App\Typle;

use RuntimeException;

/**
 * A save named a revision the workspace has moved on from (409).
 */
final class Conflict extends RuntimeException {}
