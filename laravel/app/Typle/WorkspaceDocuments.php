<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * decodeDocument of src/lib/workspaceDocument.ts: packs missing or null means none are saved.
 */
final class WorkspaceDocuments
{
    public static function decode(mixed $lists, mixed $packs, mixed $devices, mixed $interests): WorkspaceDocument
    {
        $decodedLists = WordLists::decodeLists($lists);
        $decodedDevices = Devices::decodeDevices($devices === null || $devices === Undefined::Value ? [] : $devices);
        $decodedInterests = Interests::decodeInterests($interests);
        $decodedPacks = $packs === null || $packs === Undefined::Value ? null : WordPacks::decodePacks($packs);
        $library = WordPacks::normalizeLibrary($decodedLists, $decodedPacks);

        return new WorkspaceDocument($library->packs, $library->lists, $decodedDevices, $decodedInterests);
    }
}
