<?php

declare(strict_types=1);

namespace App\Typle;

use stdClass;

/**
 * src/lib/wordPacks.ts, src/lib/workspaceDocument.ts (unique, fit) and the counts of src/lib/listOverview.ts.
 */
final class WordPacks
{
    private const EDGE = '/\A(?:'.Js::SPACE.'|・)+|(?:'.Js::SPACE.'|・)+\z/u';

    private const LABELS = ['group', 'category', 'tags'];

    public static function nameKey(string $name): string
    {
        return Js::lower(Js::trim($name));
    }

    /**
     * unique of workspaceDocument.ts: wanted, or its first variant whose lowercase is not taken yet.
     *
     * @param  array<string, true>  $taken
     */
    public static function unique(array &$taken, string $wanted, callable $variant): string
    {
        $value = $wanted;
        $count = 2;
        while (isset($taken[Js::lower($value)])) {
            $value = $variant($count);
            $count++;
        }
        $taken[Js::lower($value)] = true;

        return $value;
    }

    /** fit of workspaceDocument.ts: name cut to leave room for suffix, without splitting a surrogate pair. */
    public static function fit(string $name, string $suffix, int $limit = Limits::MAX_NAME): string
    {
        return Js::trimEnd(Js::sliceTo($name, $limit - Js::length($suffix))).$suffix;
    }

    private static function numbered(string $name): callable
    {
        return static fn (int $count): string => self::fit($name, "（{$count}）");
    }

    private static function levelPath(stdClass $list): array
    {
        $levels = Value::prop($list, 'levels');

        return is_array($levels) && $levels !== [] ? $levels : [$list->name];
    }

    private static function pathsOverlap(array $a, array $b): bool
    {
        [$short, $long] = count($a) <= count($b) ? [$a, $b] : [$b, $a];
        foreach ($short as $i => $level) {
            if ($level !== $long[$i]) {
                return false;
            }
        }

        return true;
    }

    private static function bare(string $level): string
    {
        return preg_replace(self::EDGE, '', $level);
    }

    private static function joinLevels(array $levels): ?string
    {
        $parts = array_values(array_filter(array_map(self::bare(...), $levels), static fn (string $level) => $level !== ''));
        $joined = self::bare(self::fit(implode('・', $parts), '', Limits::MAX_LEVEL));

        return $joined === '' ? null : $joined;
    }

    private static function collapseLevels(stdClass $list): stdClass
    {
        $levels = Value::prop($list, 'levels');
        if (! is_array($levels) || count($levels) <= 1) {
            return $list;
        }
        $joined = self::joinLevels($levels);

        return $joined !== null ? Value::merge($list, ['levels' => [$joined]]) : Value::except($list, 'levels');
    }

    private static function separateLevels(Library $library): Library
    {
        $lists = array_map(self::collapseLevels(...), $library->lists);
        $names = [];
        foreach ($lists as $list) {
            $names[$list->packId][self::nameKey($list->name)] = true;
        }
        $accepted = [];
        $separated = [];
        foreach ($lists as $list) {
            $packId = $list->packId;
            $levels = Value::prop($list, 'levels');
            $hasLevels = is_array($levels) && $levels !== [];
            $base = self::levelPath($list);
            $path = $base;
            $name = $list->name;
            $nameTaken = static function () use (&$path, $hasLevels, $name, $names, $packId): bool {
                return ! $hasLevels && self::nameKey($path[0]) !== self::nameKey($name) && isset($names[$packId][self::nameKey($path[0])]);
            };
            $count = 2;
            while (Value::any($accepted[$packId] ?? [], static fn (array $other) => self::pathsOverlap($other, $path)) || $nameTaken()) {
                $path[0] = self::fit($base[0], "（{$count}）", $hasLevels ? Limits::MAX_LEVEL : Limits::MAX_LIST_NAME);
                $count++;
            }
            $accepted[$packId][] = $path;
            if ($path[0] === $base[0]) {
                $separated[] = $list;
            } elseif ($hasLevels) {
                $separated[] = Value::merge($list, ['levels' => $path]);
            } else {
                $names[$packId][self::nameKey($path[0])] = true;
                $separated[] = Value::merge($list, ['name' => $path[0]]);
            }
        }

        return new Library($library->packs, $separated);
    }

    /** A pack with description, tags and origin.followed saved as null left out (withoutNulls). */
    private static function withoutNulls(mixed $pack): mixed
    {
        if (! $pack instanceof stdClass) {
            return $pack;
        }
        $origin = Value::prop($pack, 'origin');
        $cleanOrigin = $origin instanceof stdClass && property_exists($origin, 'followed') && $origin->followed === null
            ? Value::except($origin, 'followed')
            : $origin;
        $result = Value::except($pack, 'description', 'tags', 'origin');
        if (Value::prop($pack, 'description') !== null && property_exists($pack, 'description')) {
            $result->description = $pack->description;
        }
        if (Value::prop($pack, 'tags') !== null && property_exists($pack, 'tags')) {
            $result->tags = $pack->tags;
        }
        if (property_exists($pack, 'origin')) {
            $result->origin = $cleanOrigin;
        }

        return $result;
    }

    /** @return list<stdClass> */
    public static function decodePacks(mixed $value): array
    {
        if (! is_array($value)) {
            throw new AppError('savedDataCorrupted');
        }

        $packs = array_map(self::withoutNulls(...), $value);
        $ids = [];
        foreach ($packs as $pack) {
            if (! Value::truthy($pack) || ! $pack instanceof stdClass || ! is_string(Value::prop($pack, 'id')) || $pack->id === '' || isset($ids[$pack->id]) ||
                ! is_string(Value::prop($pack, 'name')) || ! is_string(Value::prop($pack, 'createdAt')) ||
                (property_exists($pack, 'description') && ! is_string($pack->description)) ||
                (property_exists($pack, 'category') && ! is_string($pack->category)) ||
                (property_exists($pack, 'tags') && (! is_array($pack->tags) || Value::any($pack->tags, static fn ($tag) => ! is_string($tag)))) ||
                (property_exists($pack, 'origin') && self::badOrigin($pack->origin))) {
                throw new AppError('savedDataCorrupted');
            }

            $ids[$pack->id] = true;
        }

        return $packs;
    }

    private static function badOrigin(mixed $origin): bool
    {
        $followed = Value::prop($origin, 'followed');
        $version = Value::prop($origin, 'version');

        return ! Value::truthy($origin) || ! is_string(Value::prop($origin, 'catalogId')) ||
            ! (is_int($version) || is_float($version)) || ($followed !== Undefined::Value && $followed !== true);
    }

    /**
     * normalizeLabels of wordLists.ts: category and tags trimmed, empty and repeated tags dropped, absent when none.
     *
     * @return array<string, mixed>
     */
    private static function normalizeLabels(mixed $category, ?array $tags): array
    {
        $name = is_string($category) ? Js::trim($category) : '';
        $seen = [];
        $kept = [];
        foreach ($tags ?? [] as $tag) {
            $value = Js::trim($tag);
            if ($value === '' || isset($seen[Js::lower($value)])) {
                continue;
            }
            $seen[Js::lower($value)] = true;
            $kept[] = $value;
        }
        $result = [];
        if ($name !== '') {
            $result['category'] = $name;
        }
        if ($kept !== []) {
            $result['tags'] = $kept;
        }

        return $result;
    }

    private static function strings(mixed $value): ?array
    {
        return is_array($value) ? $value : null;
    }

    private static function withoutLabels(stdClass $list): stdClass
    {
        return Value::except($list, ...self::LABELS);
    }

    private static function labelsOf(mixed $category, array $tags): array
    {
        $labels = self::normalizeLabels($category, $tags);
        if (isset($labels['tags'])) {
            $labels['tags'] = array_slice($labels['tags'], 0, Limits::MAX_TAGS);
        }

        return $labels;
    }

    private static function groupOf(stdClass $list): ?string
    {
        $group = Value::prop($list, 'group');

        return is_string($group) ? Js::trim($group) : null;
    }

    /** Makes packs of lists written with groups (convertLibrary), as the official lists are made into packs. */
    public static function convertLibrary(array $lists): Library
    {
        $packs = [];
        $packOf = [];
        $names = [];
        $packIds = [];
        foreach ($lists as $list) {
            $group = self::groupOf($list);
            $grouped = $group !== null && $group !== '';
            $key = $grouped ? "group:{$group}" : "list:{$list->id}";
            if (isset($packOf[$key])) {
                $packIds[] = $packOf[$key]->id;

                continue;
            }

            $members = $grouped ? array_values(array_filter($lists, static fn ($other) => self::groupOf($other) === $group)) : [$list];
            $oldest = array_reduce(array_slice($members, 1), static fn ($a, $b) => JsDate::parse($b->createdAt) < JsDate::parse($a->createdAt) ? $b : $a, $members[0]);
            $wanted = $grouped ? $group : Js::trim($list->name);
            $pack = Value::object(['id' => "pack-{$list->id}", 'name' => self::unique($names, $wanted, self::numbered($wanted))]);
            $tags = [];
            foreach ($members as $member) {
                array_push($tags, ...(self::strings(Value::prop($member, 'tags')) ?? []));
            }
            foreach (self::labelsOf(Value::prop($list, 'category'), $tags) as $label => $item) {
                $pack->{$label} = $item;
            }
            $pack->createdAt = $oldest->createdAt;
            $packOf[$key] = $pack;
            $packs[] = $pack;
            $packIds[] = $pack->id;
        }
        $placed = [];
        foreach ($lists as $i => $list) {
            $placed[] = Value::merge(self::withoutLabels($list), ['packId' => $packIds[$i]]);
        }

        return self::separateLevels(new Library($packs, $placed));
    }

    /** Keeps the rules of ownership for data an older app saved: every list in one pack, no empty packs, no repeated names. */
    public static function repairLibrary(Library $library): Library
    {
        $packs = array_map(static fn (stdClass $pack) => clone $pack, $library->packs);
        $byId = [];
        foreach ($packs as $pack) {
            $byId[$pack->id] = $pack;
        }
        $placed = [];
        foreach ($library->lists as $list) {
            $packId = Value::prop($list, 'packId');
            if (is_string($packId) && isset($byId[$packId])) {
                $placed[] = $list;

                continue;
            }

            $id = "pack-{$list->id}";
            if (! isset($byId[$id])) {
                $pack = Value::object(['id' => $id, 'name' => Js::trim($list->name), 'createdAt' => $list->createdAt]);
                $byId[$id] = $pack;
                $packs[] = $pack;
            }
            $placed[] = Value::merge($list, ['packId' => $id]);
        }
        // Labels an older app left on a list move to the pack only while the pack has none.
        $lists = [];
        foreach ($placed as $list) {
            $pack = $byId[$list->packId];
            $packTags = Value::prop($pack, 'tags');
            if (! Value::truthy(Value::prop($pack, 'category')) && (! is_array($packTags) || $packTags === [])) {
                foreach (self::normalizeLabels(Value::prop($list, 'category'), self::strings(Value::prop($list, 'tags'))) as $label => $item) {
                    $pack->{$label} = $item;
                }
            }
            $lists[] = self::withoutLabels($list);
        }
        $used = [];
        foreach ($lists as $list) {
            $used[$list->packId] = true;
        }
        $names = [];
        $kept = [];
        foreach ($packs as $pack) {
            if (! isset($used[$pack->id])) {
                continue;
            }
            $name = Js::trim($pack->name);
            $kept[] = Value::merge($pack, ['name' => self::unique($names, $name, self::numbered($name))]);
        }
        $listNames = [];
        $named = [];
        foreach ($lists as $list) {
            $name = Js::trim($list->name);
            $listNames[$list->packId] ??= [];
            $named[] = Value::merge($list, ['name' => self::unique($listNames[$list->packId], $name, self::numbered($name))]);
        }

        return self::separateLevels(new Library($kept, $named));
    }

    public static function normalizeLibrary(array $lists, ?array $packs): Library
    {
        return self::repairLibrary(new Library($packs ?? [], $lists));
    }

    /** Refuses a library that is not in the saved shape (PlainError), or whose levels a save may not have (AppError). */
    public static function validateLibrary(Library $library): void
    {
        $ids = [];
        $names = [];
        foreach ($library->packs as $pack) {
            $name = self::nameKey($pack->name);
            $id = $pack->id;
            $tags = self::strings(Value::prop($pack, 'tags'));
            $description = Value::prop($pack, 'description');
            $category = Value::prop($pack, 'category');
            if ($id === '' || isset($ids[$id]) || $name === '' || Js::length($pack->name) > Limits::MAX_PACK_NAME || isset($names[$name]) ||
                Js::length(is_string($description) ? $description : '') > Limits::MAX_PACK_DESCRIPTION ||
                Js::length(is_string($category) ? $category : '') > Limits::MAX_CATEGORY ||
                count($tags ?? []) > Limits::MAX_TAGS || Value::any($tags ?? [], static fn ($tag) => Js::length($tag) > Limits::MAX_TAG)) {
                throw new PlainError('Invalid packs');
            }

            $ids[$id] = true;
            $names[$name] = true;
        }
        $listNames = [];
        foreach ($library->lists as $list) {
            $packId = Value::prop($list, 'packId');
            $packKey = is_string($packId) ? $packId : '';
            $listNames[$packKey] ??= [];
            $name = self::nameKey($list->name);
            if (! is_string($packId) || $packId === '' || ! isset($ids[$packId]) || isset($listNames[$packKey][$name]) ||
                Value::any(self::LABELS, static fn (string $key) => property_exists($list, $key))) {
                throw new PlainError('Invalid packs');
            }

            $listNames[$packKey][$name] = true;
        }
        if (count($ids) !== count($listNames)) {
            throw new PlainError('Invalid packs');
        }

        $paths = [];
        foreach ($library->lists as $list) {
            $levels = Value::prop($list, 'levels');
            if (is_array($levels) && count($levels) > Limits::MAX_LEVELS) {
                throw new AppError('tooManyLevels');
            }

            $path = self::levelPath($list);
            if (Value::any($paths[$list->packId] ?? [], static fn (array $other) => self::pathsOverlap($other, $path))) {
                throw new AppError('levelsConflict');
            }

            $paths[$list->packId][] = $path;
        }
    }

    private static function followedIds(array $packs): array
    {
        $ids = [];
        foreach ($packs as $pack) {
            if (Value::prop(Value::prop($pack, 'origin'), 'followed') === true) {
                $ids[$pack->id] = true;
            }
        }

        return $ids;
    }

    public static function ownListCount(Library $library): int
    {
        $followed = self::followedIds($library->packs);

        return count(array_filter($library->lists, static function ($list) use ($followed): bool {
            $packId = Value::prop($list, 'packId');

            return ! (is_string($packId) && isset($followed[$packId]));
        }));
    }

    public static function followedPackCount(array $packs): int
    {
        return count(self::followedIds($packs));
    }
}
