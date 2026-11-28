<?php

declare(strict_types=1);

namespace App\Typle;

use stdClass;

/**
 * src/lib/records.ts: the checks and changes of a list's practice records.
 */
final class Records
{
    private const RECORD_ID = '/\A[A-Za-z0-9_-]+\z/';

    public static function missedWords(mixed $value, int $limit = Limits::MAX_MISSED_WORDS): bool
    {
        return is_array($value) && count($value) <= $limit && Value::all($value, static function ($item): bool {
            $word = Value::prop($item, 'word');
            $count = Value::prop($item, 'count');

            return Value::objectLike($item) &&
                (! $item instanceof stdClass || Value::all(Value::keys($item), static fn (string $key) => $key === 'word' || $key === 'count')) &&
                is_string($word) && $word !== '' && Js::length($word) <= Limits::MAX_WORD_TEXT &&
                Js::safeInteger($count) && $count >= 1;
        });
    }

    public static function weakReview(mixed $value): bool
    {
        if (! $value instanceof stdClass) {
            return false;
        }
        $date = Value::prop($value, 'date');

        return Value::all(Value::keys($value), static fn (string $key) => $key === 'date' || $key === 'missed') &&
            is_string($date) && is_finite(JsDate::parse($date)) &&
            (! Value::has($value, 'missed') || self::missedWords($value->missed, Limits::MAX_WEAK_REVIEW_MISSED));
    }

    private static function counted(mixed $value, int $min): bool
    {
        return Js::finiteNumber($value) && Js::safeInteger($value) && $value >= $min;
    }

    private static function set(mixed $input, string $key): bool
    {
        return Value::prop($input, $key) !== Undefined::Value;
    }

    /**
     * Adds a run to a list (addRecord). A run with the id of one the list already has is not added again: that saved
     * record is returned with added false.
     *
     * @return array{lists: list<stdClass>, record: mixed, added: bool}
     */
    public static function addRecord(array $lists, array $devices, string $listId, mixed $input): array
    {
        $list = Value::find($lists, static fn ($item) => Value::prop($item, 'id') === $listId) ?? throw new AppError('recordListDeleted');
        $dateText = Value::prop($input, 'date');
        $time = is_string($dateText) ? JsDate::parse($dateText) : NAN;
        $year = JsDate::utcYear($time);
        $run = Value::prop($input, 'time');
        $accuracy = Value::prop($input, 'accuracy');
        $mistakes = Value::prop($input, 'mistakes');
        $wordCount = Value::prop($input, 'wordCount');
        $mode = Value::prop($input, 'mode');
        $id = Value::prop($input, 'id');
        $missed = Value::prop($input, 'missed');
        if (! Js::finiteNumber($run) || $run < 0 || ! Js::finiteNumber($accuracy) || $accuracy < 0 || $accuracy > 100 ||
            ! ($year >= 0 && $year <= 9999) || ! self::counted($mistakes, 0) || (self::set($input, 'wordCount') && ! self::counted($wordCount, 1)) ||
            (self::set($input, 'mode') && $mode !== 'recall' && $mode !== 'writing') || ! is_string($id) ||
            Js::length($id) > Limits::MAX_RECORD_ID || ! preg_match(self::RECORD_ID, $id) ||
            (self::set($input, 'missed') && $missed !== null && ! self::missedWords($missed))) {
            throw new AppError('invalidRecord');
        }

        $records = $list->records;
        $saved = Value::find($records, static fn ($record) => Value::prop($record, 'id') === $id);
        if ($saved !== null) {
            return ['lists' => $lists, 'record' => $saved, 'added' => false];
        }
        if (count($records) >= Limits::MAX_RECORDS) {
            throw new AppError('recordLimit');
        }

        $device = null;
        if (self::set($input, 'device')) {
            $sent = $input->device;
            if (self::set($input, 'deviceId') || ! Devices::isDevice($sent)) {
                throw new AppError('invalidRecord');
            }
            $device = Value::object(['id' => $sent->id, 'name' => $sent->name, 'layout' => $sent->layout]);
        } elseif (self::set($input, 'deviceId')) {
            $deviceId = $input->deviceId;
            $device = $deviceId === Devices::DEFAULT_DEVICE_ID
                ? Devices::defaultDevice(Catalog::defaultDeviceName())
                : Value::find($devices, static fn ($item) => Value::prop($item, 'id') === $deviceId);
            if ($device === null) {
                throw new AppError('deviceNotFound');
            }
        }
        $record = Value::object([
            'id' => $id, 'time' => $run, 'date' => JsDate::toIso($time), 'mistakes' => $mistakes, 'accuracy' => $accuracy,
            'wordCount' => self::set($input, 'wordCount') ? $wordCount : count($list->words),
        ]);
        if (self::set($input, 'mode')) {
            $record->mode = $mode;
        }
        if ($device !== null) {
            $record->device = clone $device;
        }
        if (is_array($missed) && $missed !== []) {
            $record->missed = array_map(static fn ($item) => Value::object(['word' => $item->word, 'count' => $item->count]), $missed);
        }
        $nextLists = array_map(static fn ($item) => $item === $list ? Value::merge($item, ['records' => [...$records, $record]]) : $item, $lists);

        return ['lists' => $nextLists, 'record' => $record, 'added' => true];
    }

    /**
     * Deletes a run from a list (deleteRecord); null when it, or its list, is already gone.
     *
     * @return list<stdClass>|null
     */
    public static function deleteRecord(array $lists, string $listId, string $recordId): ?array
    {
        $list = Value::find($lists, static fn ($item) => Value::prop($item, 'id') === $listId);
        $records = $list === null ? null : Value::prop($list, 'records');
        if ($recordId === '' || ! is_array($records) || ! Value::any($records, static fn ($record) => Value::prop($record, 'id') === $recordId)) {
            return null;
        }

        return array_map(static fn ($item) => $item === $list
            ? Value::merge($item, ['records' => array_values(array_filter($records, static fn ($record) => Value::prop($record, 'id') !== $recordId))])
            : $item, $lists);
    }
}
