<?php

declare(strict_types=1);

namespace App\Typle;

use stdClass;
use Throwable;

/**
 * The change a PUT /api/workspace body asks for, and what a save writes (readWorkspaceChange and encodeChange of
 * src/server/workspace.ts).
 */
final class WorkspaceChanges
{
    public const MAX_LISTS_BYTES = 896 << 10;

    private const INVALID = ['refused', 'invalidWorkspace'];

    /** Whether value holds a string (or key) jsonb cannot keep: one with U+0000 or one that is not valid UTF-8. */
    public static function unstorable(mixed $value): bool
    {
        if (is_string($value)) {
            return str_contains($value, "\0") || ! mb_check_encoding($value, 'UTF-8');
        }
        if (is_array($value)) {
            return Value::any($value, self::unstorable(...));
        }
        if ($value instanceof stdClass) {
            foreach ($value as $key => $item) {
                if (self::unstorable((string) $key) || self::unstorable($item)) {
                    return true;
                }
            }
        }

        return false;
    }

    /**
     * ['ok', revision, change], or ['refused', key] (invalidWorkspace or levelsConflict).
     *
     * @return array{'ok', int, WorkspaceChange}|array{'refused', string}
     */
    public static function read(stdClass $fields): array
    {
        foreach (['lists', 'packs', 'devices'] as $name) {
            if (property_exists($fields, $name) && $fields->{$name} === null) {
                return self::INVALID;
            }
        }
        foreach (['lists', 'packs', 'devices', 'interests'] as $name) {
            if (property_exists($fields, $name) && self::unstorable($fields->{$name})) {
                return self::INVALID;
            }
        }

        $interests = Value::prop($fields, 'interests');
        $rest = Value::except($fields, 'accountId', 'interests');
        $several = [];
        $lists = Value::prop($rest, 'lists');
        // A save may hold up to MAX_SAVED_LEVELS levels of a list sent with its packs, which the checks of one level let pass.
        if (is_array($lists) && property_exists($rest, 'packs')) {
            foreach ($lists as $index => $item) {
                $levels = Value::prop($item, 'levels');
                if (! is_array($levels) || count($levels) <= 1 || count($levels) > Limits::MAX_SAVED_LEVELS ||
                    Value::any($levels, static fn ($level) => ! is_string($level) || Js::length($level) > Limits::MAX_LEVEL)) {
                    continue;
                }
                $several[$index] = $levels;
                $lists[$index] = Value::merge($item, ['levels' => ["\0several-{$index}"]]);
            }
        }
        $body = clone $rest;
        if ($lists !== Undefined::Value) {
            $body->lists = $lists;
        }
        if ($interests !== Undefined::Value && $interests !== null) {
            $body->interests = $interests;
        }
        try {
            [$revision, $change] = WorkspaceRequest::validate($body);
        } catch (AppError $error) {
            return $error->key === 'levelsConflict' ? ['refused', 'levelsConflict'] : self::INVALID;
        } catch (Throwable) {
            return self::INVALID;
        }
        if ($change->lists !== null && $several !== []) {
            foreach ($change->lists as $index => $list) {
                if (isset($several[$index])) {
                    $change->lists[$index] = Value::merge($list, ['levels' => $several[$index]]);
                }
            }
        }
        if (property_exists($fields, 'interests') && $interests === null) {
            $change->interests = [];
        }

        return ['ok', $revision, $change];
    }

    /** encodeChange: the repaired lists and packs a save writes, without followed words and records, and their rows. */
    public static function encode(WorkspaceChange $change): Encoded
    {
        if ($change->lists === null) {
            return new Encoded($change->devices, $change->interests, null);
        }

        $repaired = Catalog::followed(WordPacks::repairLibrary(new Library($change->packs ?? [], $change->lists)));
        $rows = RecordRows::rows($repaired->lists);
        $slim = FollowedPacks::withoutFollowedWords($repaired);
        // A change the app could not read back is never written.
        try {
            $document = WorkspaceDocuments::decode($slim->lists, $slim->packs, $change->devices ?? [], $change->interests ?? Undefined::Value);
            Catalog::followed($document->library());
        } catch (Throwable) {
            throw new AppError('invalidWorkspace');
        }
        $size = Js::utf8Bytesize(JsonValue::generate($slim->lists)) + Js::utf8Bytesize(JsonValue::generate($slim->packs));

        return new Encoded($change->devices, $change->interests, new EncodedLibrary(
            array_map(static fn (stdClass $list) => Value::merge($list, ['records' => []]), $slim->lists), $slim->packs, $rows,
            WordPacks::ownListCount($repaired), WordPacks::followedPackCount($repaired->packs), $size,
        ));
    }
}
