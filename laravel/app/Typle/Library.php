<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * A library: packs and lists in the saved shape, each an object as saved (unknown fields kept, in their order).
 */
final class Library
{
    /**
     * @param  list<\stdClass>  $packs
     * @param  list<\stdClass>  $lists
     */
    public function __construct(public array $packs, public array $lists) {}
}
