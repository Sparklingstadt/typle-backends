<?php

declare(strict_types=1);

namespace App\Casts;

use App\Typle\JsonValue;
use Illuminate\Contracts\Database\Eloquent\CastsAttributes;
use Illuminate\Database\Eloquent\Model;

/**
 * A jsonb column read and written as JSON.parse and JSON.stringify do (App\Typle\JsonValue), not as Laravel's json cast
 * does (which reads objects as arrays, and writes numbers and slashes otherwise). A string written is JSON text already
 * (a record's data, written once for its row).
 */
class Json implements CastsAttributes
{
    public function get(Model $model, string $key, mixed $value, array $attributes): mixed
    {
        return is_string($value) ? JsonValue::parse($value) : $value;
    }

    public function set(Model $model, string $key, mixed $value, array $attributes): ?string
    {
        return $value === null ? null : (is_string($value) ? $value : JsonValue::generate($value));
    }
}
