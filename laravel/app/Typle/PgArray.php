<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * PostgreSQL's text form of an array (what PG::TextEncoder::Array writes for the Rails backend, and pgx for Go's): the
 * parameters of the statements that take a column of values with unnest.
 */
final class PgArray
{
    /** @param  list<string|int|float>  $values */
    public static function encode(array $values): string
    {
        return '{'.implode(',', array_map(static fn ($value) => is_string($value)
            ? '"'.str_replace(['\\', '"'], ['\\\\', '\\"'], $value).'"'
            : (string) $value, $values)).'}';
    }
}
