<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * The parts of a save as written: devices and interests (null to keep), and the library (null to keep).
 */
final readonly class Encoded
{
    public function __construct(public ?array $devices, public ?array $interests, public ?EncodedLibrary $library) {}
}
