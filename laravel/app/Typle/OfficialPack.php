<?php

declare(strict_types=1);

namespace App\Typle;

use stdClass;

/**
 * An official pack as the catalog holds it, in the saved shape.
 */
final readonly class OfficialPack
{
    /** @param  list<stdClass>  $lists */
    public function __construct(public string $catalogId, public int|float $version, public stdClass $pack, public array $lists) {}
}
