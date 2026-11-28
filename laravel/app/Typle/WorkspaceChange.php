<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * A change of a save; null parts stay as they are.
 */
final class WorkspaceChange
{
    public function __construct(public ?array $lists, public ?array $packs = null, public ?array $devices = null, public ?array $interests = null) {}
}
