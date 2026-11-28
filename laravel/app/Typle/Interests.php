<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * src/lib/interests.ts: the ids of the options, in the order they are shown and saved.
 */
final class Interests
{
    public const OPTIONS = [
        ['language', ['zh', 'ru', 'ko', 'en', 'es', 'fr', 'de', 'cs', 'ja']],
        ['programming', ['go', 'kotlin', 'java', 'typescript', 'php', 'ruby', 'sql', 'linux', 'vim', 'test', 'node', 'shell', 'cloud']],
    ];

    /** The interests of a user who has saved none: every option (tests/fixtures/default-interests.json). */
    public static function defaults(): array
    {
        static $defaults = null;
        if ($defaults === null) {
            $defaults = [];
            foreach (self::OPTIONS as [$id, $children]) {
                $defaults[] = $id;
                foreach ($children as $child) {
                    $defaults[] = "{$id}/{$child}";
                }
            }
        }

        return $defaults;
    }

    public static function decodeInterests(mixed $value): array
    {
        if ($value === null || $value === Undefined::Value) {
            return self::defaults();
        }
        if (! is_array($value) || count($value) > Limits::MAX_INTERESTS ||
            Value::any($value, static fn ($item) => ! is_string($item) || $item === '' || Js::length($item) > Limits::MAX_INTEREST_LENGTH) ||
            ! Value::unique($value)) {
            throw new AppError('savedDataCorrupted');
        }

        return $value;
    }
}
