<?php

declare(strict_types=1);

namespace App\Typle;

use stdClass;

/**
 * src/lib/followedPacks.ts: keeping followed official packs in line with the catalog.
 */
final class FollowedPacks
{
    private static function followed(stdClass $pack): bool
    {
        return Value::prop(Value::prop($pack, 'origin'), 'followed') === true;
    }

    /** The lists as they are saved: those of a followed pack without their words (withoutFollowedWords). */
    public static function withoutFollowedWords(Library $library): Library
    {
        $followed = [];
        foreach ($library->packs as $pack) {
            if (self::followed($pack)) {
                $followed[$pack->id] = true;
            }
        }

        return new Library($library->packs, array_map(static function (stdClass $list) use ($followed): stdClass {
            $packId = Value::prop($list, 'packId');

            return is_string($packId) && isset($followed[$packId]) && Value::truthy(Value::prop($list, 'origin'))
                ? Value::merge($list, ['words' => []])
                : $list;
        }, $library->lists));
    }

    /**
     * followOfficial: removes copies from before v0.98.0 and makes each followed pack equal to its official pack.
     *
     * @param  list<OfficialPack>  $official
     */
    public static function followOfficial(Library $library, array $official): Library
    {
        $catalog = [];
        foreach ($official as $entry) {
            $catalog[$entry->catalogId] = $entry;
        }
        $inCatalog = static fn (mixed $id): bool => is_string($id) && isset($catalog[$id]);
        $kept = array_values(array_filter($library->packs, static function (stdClass $pack) use ($inCatalog): bool {
            $origin = Value::prop($pack, 'origin');
            if (Value::truthy($origin)) {
                return self::followed($pack) && $inCatalog(Value::prop($origin, 'catalogId'));
            }

            return ! $inCatalog($pack->id);
        }));
        $packs = [];
        $synced = [];
        foreach ($kept as $pack) {
            if (! self::followed($pack)) {
                $packs[] = $pack;

                continue;
            }
            $entry = $catalog[$pack->origin->catalogId];
            $rest = Value::except($pack, 'description', 'category', 'tags');
            foreach (['description', 'category'] as $key) {
                $item = Value::prop($entry->pack, $key);
                if (is_string($item) && $item !== '') {
                    $rest->{$key} = $item;
                }
            }
            $tags = Value::prop($entry->pack, 'tags');
            if (is_array($tags) && $tags !== []) {
                $rest->tags = $tags;
            }
            $packs[] = $rest;
            $packId = $pack->id;
            $mine = array_values(array_filter($library->lists, static fn ($list) => Value::prop($list, 'packId') === $packId));
            $synced[$packId] = array_map(static function (stdClass $item) use ($mine, $packId): stdClass {
                $found = Value::find($mine, static fn ($list) => Value::prop(Value::prop($list, 'origin'), 'listId') === $item->id);
                $base = $found ?? Value::object([
                    'id' => "{$packId}:{$item->id}", 'packId' => $packId, 'records' => [], 'createdAt' => $item->createdAt,
                    'origin' => Value::object(['listId' => $item->id]),
                ]);
                $list = Value::except($base, 'levels', 'writing');
                $list->name = $item->name;
                $levels = Value::prop($item, 'levels');
                if (is_array($levels) && $levels !== []) {
                    $list->levels = $levels;
                }
                $writing = Value::prop($item, 'writing');
                if ($writing instanceof stdClass) {
                    $list->writing = clone $writing;
                }
                $list->words = array_map(static fn ($word) => is_object($word) ? clone $word : $word, $item->words);

                return $list;
            }, $entry->lists);
        }
        $byId = [];
        foreach ($packs as $pack) {
            $byId[$pack->id] = $pack;
        }
        $placed = [];
        $lists = [];
        foreach ($library->lists as $list) {
            $packId = Value::prop($list, 'packId');
            $pack = is_string($packId) ? ($byId[$packId] ?? null) : null;
            if ($pack === null) {
                continue;
            }
            $id = $pack->id;
            if (! isset($synced[$id])) {
                $lists[] = $list;

                continue;
            }
            if (isset($placed[$id])) {
                continue;
            }
            $placed[$id] = true;
            array_push($lists, ...$synced[$id]);
        }

        return new Library($packs, $lists);
    }
}
