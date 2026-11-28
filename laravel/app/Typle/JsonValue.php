<?php

declare(strict_types=1);

namespace App\Typle;

use stdClass;

/**
 * JSON as JSON.parse and JSON.stringify treat it (rails/app/lib/typle/json_value.rb): a repeated key keeps its last
 * value (in the place of its first), and a number is one Number, so 1500.0 is 1500 (an int here, written "1500" as
 * JSON.stringify writes it). PHP's parser refuses a lone surrogate in a string, which JSON.parse takes, as Ruby's does;
 * every body that holds one is refused all the same (a string jsonb cannot keep). Objects nest at most 100 deep, as
 * Ruby's parser allows.
 */
final class JsonValue
{
    public const MAX_EXACT = 9007199254740992;

    /** Arrays and objects nested 100 deep, and no deeper (json_decode counts one level more than Ruby's max_nesting). */
    private const DEPTH = 101;

    private const FLAGS = JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_LINE_TERMINATORS;

    /** JSON.parse(text): JsonException for what it refuses. */
    public static function parse(string $text): mixed
    {
        return self::normalize(json_decode($text, false, self::DEPTH, JSON_THROW_ON_ERROR));
    }

    /**
     * JSON.stringify(value): a non-finite number is null, and a float is written as Number#toString writes it. PHP's
     * encoder writes the same as JSON.stringify for every value but some floats (those it writes with an exponent, or
     * -0), so it writes the value unless one of those is in it.
     */
    public static function generate(mixed $value): string
    {
        if (self::plain($value)) {
            return json_encode($value, self::FLAGS | JSON_THROW_ON_ERROR);
        }

        return self::write($value);
    }

    /** Whether json_encode writes value as JSON.stringify does: no float it would write otherwise. */
    private static function plain(mixed $value): bool
    {
        if (is_float($value)) {
            $magnitude = abs($value);

            return is_finite($value) && $magnitude >= 1e-4 && $magnitude < 1e17;
        }
        if (is_array($value) || $value instanceof stdClass) {
            foreach ($value as $item) {
                if (! is_string($item) && ! self::plain($item)) {
                    return false;
                }
            }
        }

        return true;
    }

    private static function write(mixed $value): string
    {
        if (is_float($value)) {
            return is_finite($value) ? Js::number($value) : 'null';
        }
        if ($value instanceof stdClass) {
            $parts = [];
            foreach ($value as $key => $item) {
                $parts[] = json_encode((string) $key, self::FLAGS | JSON_THROW_ON_ERROR).':'.self::write($item);
            }

            return '{'.implode(',', $parts).'}';
        }
        if (is_array($value)) {
            if (! array_is_list($value)) {
                return self::write(Value::object($value));
            }

            return '['.implode(',', array_map(self::write(...), $value)).']';
        }

        return json_encode($value, self::FLAGS | JSON_THROW_ON_ERROR);
    }

    /** value with every float that is a whole number within 2^53 as an int, and every larger int as a float. */
    public static function normalize(mixed $value): mixed
    {
        if ($value instanceof stdClass) {
            foreach ($value as $key => $item) {
                if (! is_string($item) && $item !== null && ! is_bool($item)) {
                    $value->{$key} = self::normalize($item);
                }
            }

            return $value;
        }
        if (is_array($value)) {
            foreach ($value as $index => $item) {
                if (! is_string($item) && $item !== null && ! is_bool($item)) {
                    $value[$index] = self::normalize($item);
                }
            }

            return $value;
        }
        if (is_float($value)) {
            return is_finite($value) && $value == floor($value) && abs($value) <= self::MAX_EXACT ? (int) $value : $value;
        }
        if (is_int($value) && ($value > self::MAX_EXACT || $value < -self::MAX_EXACT)) {
            return (float) $value;
        }

        return $value;
    }
}
