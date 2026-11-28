<?php

declare(strict_types=1);

namespace App\Typle;

use stdClass;

/**
 * The port of src/lib and src/server that the compared endpoints need (rails/app/lib/typle is the same port). Values are
 * JSON as JsonValue reads it: a JSON object is a stdClass (its keys strings, in their order, and shared by reference as a
 * JavaScript object is), a JSON array a PHP list, and strings, ints, floats, true, false and null. A key that is not
 * there is JavaScript's undefined (Undefined::Value, which prop returns).
 */
final class Value
{
    /** value[key] as JavaScript reads a property: undefined when value is not an object or has no such key. */
    public static function prop(mixed $value, string $key): mixed
    {
        return $value instanceof stdClass && property_exists($value, $key) ? $value->{$key} : Undefined::Value;
    }

    /** Whether value is an object that has key (Ruby's Hash#key?). */
    public static function has(mixed $value, string $key): bool
    {
        return $value instanceof stdClass && property_exists($value, $key);
    }

    public static function isObject(mixed $value): bool
    {
        return $value instanceof stdClass;
    }

    /** An object or an array: what typeof gives "object" for, but null. */
    public static function objectLike(mixed $value): bool
    {
        return $value instanceof stdClass || is_array($value);
    }

    /** Whether JavaScript takes value as true in a condition (an empty array or object is true). */
    public static function truthy(mixed $value): bool
    {
        if ($value === null || $value === Undefined::Value || $value === false || $value === '') {
            return false;
        }
        if (is_int($value)) {
            return $value !== 0;
        }
        if (is_float($value)) {
            return $value != 0.0 && ! is_nan($value);
        }

        return true;
    }

    /** The keys of an object, as strings, in their order. */
    public static function keys(stdClass $value): array
    {
        $keys = [];
        foreach ($value as $key => $_) {
            $keys[] = (string) $key;
        }

        return $keys;
    }

    /** The number of keys of an object. */
    public static function size(stdClass $value): int
    {
        return count(get_object_vars($value));
    }

    /** An object of the pairs, in their order. */
    public static function object(array $pairs = []): stdClass
    {
        $object = new stdClass;
        foreach ($pairs as $key => $item) {
            $object->{(string) $key} = $item;
        }

        return $object;
    }

    /** A copy of value with the pairs set: an existing key keeps its place, a new one goes last (Hash#merge). */
    public static function merge(stdClass $value, array $pairs): stdClass
    {
        $copy = clone $value;
        foreach ($pairs as $key => $item) {
            $copy->{(string) $key} = $item;
        }

        return $copy;
    }

    /** A copy of value without the keys (Hash#except). */
    public static function except(stdClass $value, string ...$keys): stdClass
    {
        $copy = clone $value;
        foreach ($keys as $key) {
            unset($copy->{$key});
        }

        return $copy;
    }

    public static function deepCopy(mixed $value): mixed
    {
        if ($value instanceof stdClass) {
            $copy = new stdClass;
            foreach ($value as $key => $item) {
                $copy->{(string) $key} = self::deepCopy($item);
            }

            return $copy;
        }
        if (is_array($value)) {
            return array_map(self::deepCopy(...), $value);
        }

        return $value;
    }

    /** The first item for which test is true, or null (Enumerable#find). */
    public static function find(array $items, callable $test): mixed
    {
        foreach ($items as $item) {
            if ($test($item)) {
                return $item;
            }
        }

        return null;
    }

    public static function any(array $items, callable $test): bool
    {
        foreach ($items as $item) {
            if ($test($item)) {
                return true;
            }
        }

        return false;
    }

    public static function all(array $items, callable $test): bool
    {
        foreach ($items as $item) {
            if (! $test($item)) {
                return false;
            }
        }

        return true;
    }

    /** Whether the strings (or ints) of items are all different (Array#uniq of JSON values compared as Ruby does). */
    public static function unique(array $items): bool
    {
        $seen = [];
        foreach ($items as $item) {
            $key = self::identity($item);
            if (isset($seen[$key])) {
                return false;
            }
            $seen[$key] = true;
        }

        return true;
    }

    /** A key that tells JSON values apart as Ruby's eql? does (1 and 1.0 apart, "1" and 1 apart). */
    private static function identity(mixed $value): string
    {
        return get_debug_type($value).':'.(is_string($value) ? $value : JsonValue::generate($value));
    }
}
