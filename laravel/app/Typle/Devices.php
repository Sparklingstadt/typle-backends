<?php

declare(strict_types=1);

namespace App\Typle;

use stdClass;

/**
 * src/lib/devices.ts.
 */
final class Devices
{
    public const DEFAULT_DEVICE_ID = 'default';

    public static function defaultDevice(string $name): stdClass
    {
        return Value::object(['id' => self::DEFAULT_DEVICE_ID, 'name' => $name, 'layout' => '']);
    }

    public static function isDevice(mixed $value): bool
    {
        if (! Value::objectLike($value)) {
            return false;
        }
        $id = Value::prop($value, 'id');
        $name = Value::prop($value, 'name');
        $layout = Value::prop($value, 'layout');

        return is_string($id) && $id !== '' && Js::length($id) <= 100 &&
            is_string($name) && Js::trim($name) !== '' && Js::length($name) <= Limits::MAX_DEVICE_NAME &&
            is_string($layout) && Js::length($layout) <= Limits::MAX_DEVICE_LAYOUT;
    }

    /** @return list<stdClass> */
    public static function decodeDevices(mixed $value): array
    {
        if (! is_array($value) || count($value) > Limits::MAX_DEVICES || ! Value::all($value, self::isDevice(...)) ||
            Value::any($value, static fn ($item) => $item->id === self::DEFAULT_DEVICE_ID) ||
            ! Value::unique(array_map(static fn ($item) => $item->id, $value))) {
            throw new AppError('deviceDataInvalid');
        }

        return array_map(static fn ($item) => Value::object(['id' => $item->id, 'name' => Js::trim($item->name), 'layout' => Js::trim($item->layout)]), $value);
    }
}
