<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * A workspace as the app reads it.
 */
final class WorkspaceDocument
{
    public function __construct(public array $packs, public array $lists, public array $devices, public array $interests) {}

    public function library(): Library
    {
        return new Library($this->packs, $this->lists);
    }
}
