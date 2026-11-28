<?php

declare(strict_types=1);

namespace App\Typle;

use stdClass;

/**
 * src/lib/workspaceRequest.ts.
 */
final class WorkspaceRequest
{
    private static function tooLarge(stdClass $list): bool
    {
        $words = $list->words;
        $tags = Value::prop($list, 'tags');
        $levels = Value::prop($list, 'levels');
        $category = Value::prop($list, 'category');
        $group = Value::prop($list, 'group');

        return count($words) > Limits::MAX_WORDS || count($list->records) > Limits::MAX_RECORDS ||
            Js::length($list->name) > Limits::MAX_LIST_NAME || Js::length(is_string($category) ? $category : '') > Limits::MAX_CATEGORY ||
            (is_array($tags) ? count($tags) : 0) > Limits::MAX_TAGS || (is_array($tags) && Value::any($tags, static fn ($tag) => Js::length($tag) > Limits::MAX_TAG)) ||
            Js::length(is_string($group) ? $group : '') > Limits::MAX_GROUP ||
            (is_array($levels) ? count($levels) : 0) > Limits::MAX_SAVED_LEVELS ||
            (is_array($levels) && Value::any($levels, static fn ($level) => Js::length($level) > Limits::MAX_LEVEL)) ||
            Value::any($words, static fn ($word) => Js::length($word->input) > Limits::MAX_WORD_TEXT ||
                Js::length($word->display) > Limits::MAX_WORD_TEXT || Js::length($word->annotation) > Limits::MAX_ANNOTATION);
    }

    /**
     * validateLists: the lists read as saved data, within the limits of a save (PlainError past them).
     *
     * @return list<stdClass>
     */
    public static function validateLists(mixed $value): array
    {
        $lists = WordLists::decodeLists($value);
        if (Value::any($lists, self::tooLarge(...))) {
            throw new PlainError('Workspace too large');
        }

        return $lists;
    }

    /**
     * validateWorkspaceRequest: the revision and the change of a body.
     *
     * @return array{int, WorkspaceChange}
     */
    public static function validate(stdClass $body): array
    {
        $revision = Value::prop($body, 'revision');
        if (! Js::safeInteger($revision) || $revision < 0) {
            throw new PlainError('Invalid revision');
        }
        if (Value::has($body, 'profileId') || Value::has($body, 'profiles')) {
            throw new PlainError('Invalid request');
        }
        if (Value::has($body, 'packs') && ! Value::has($body, 'lists')) {
            throw new PlainError('Invalid request');
        }

        $lists = Value::has($body, 'lists') ? self::validateLists($body->lists) : null;
        $packs = Value::has($body, 'packs') ? WordPacks::decodePacks($body->packs) : null;
        if ($lists !== null && $packs !== null) {
            WordPacks::validateLibrary(new Library($packs, $lists));
        }
        $devices = Value::has($body, 'devices') ? Devices::decodeDevices($body->devices) : null;
        $interests = Value::has($body, 'interests') ? Interests::decodeInterests($body->interests) : null;

        return [(int) $revision, new WorkspaceChange($lists, $packs, $devices, $interests)];
    }
}
