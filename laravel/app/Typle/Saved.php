<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * A saved workspace as stored (with its records put back in its lists): what GET and PUT /api/workspace answer.
 */
final readonly class Saved
{
    public function __construct(public mixed $lists, public mixed $packs, public mixed $devices, public mixed $interests, public int $revision) {}
}
