<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * A record as typle_records keeps it. at is milliseconds since the epoch; data is the record as JSON.
 */
final readonly class RecordRow
{
    public function __construct(public string $listId, public string $recordId, public int $position, public float $at, public string $data) {}
}
