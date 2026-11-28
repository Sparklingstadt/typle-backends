<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * What a save writes of the lists and packs: the lists without records (and followed words), the packs, the records as
 * rows, the numbers of own lists and followed packs, and the size the save is held to.
 */
final readonly class EncodedLibrary
{
    /** @param  list<RecordRow>  $rows */
    public function __construct(
        public array $lists,
        public array $packs,
        public array $rows,
        public int $own,
        public int $followed,
        public int $size,
    ) {}
}
