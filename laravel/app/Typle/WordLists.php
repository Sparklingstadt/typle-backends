<?php

declare(strict_types=1);

namespace App\Typle;

use stdClass;

/**
 * src/lib/wordLists.ts and the record checks of src/lib/records.ts that reading lists needs.
 */
final class WordLists
{
    public const WRITING_LANGUAGES = ['en', 'ja', 'zh-Hans', 'zh-Hant', 'ko', 'fr', 'de', 'es', 'ru', 'cs'];

    private const LIST_NULLS = ['writing', 'tags', 'category', 'group', 'levels', 'packId'];

    private const RECORD_NULLS = ['id', 'mistakes', 'accuracy', 'wordCount', 'device', 'missed'];

    public static function inputType(mixed $value): bool
    {
        return in_array($value, ['command', 'code', 'word'], true);
    }

    /** A valid mark: an object whose only key is a known language. */
    public static function writingMark(mixed $value): bool
    {
        return $value instanceof stdClass && Value::size($value) === 1 && in_array(Value::prop($value, 'language'), self::WRITING_LANGUAGES, true);
    }

    public static function validateList(stdClass $list): void
    {
        if (Js::trim($list->name) === '') {
            throw new AppError('listNameRequired');
        }
        $words = $list->words;
        if ($words === []) {
            throw new AppError('wordsRequired');
        }
        if (Value::any($words, static fn ($word) => Js::trim($word->display) === '' || Js::trim($word->input) === '')) {
            throw new AppError('wordFieldsRequired');
        }
        if (Value::any($words, static fn ($word) => Value::has($word, 'input_type') && ! self::inputType($word->input_type))) {
            throw new AppError('savedDataCorrupted');
        }
        if (Value::has($list, 'writing') && ! self::writingMark($list->writing)) {
            throw new AppError('invalidWritingLanguage');
        }
    }

    private static function dropNulls(stdClass $value, array $names): void
    {
        foreach ($names as $name) {
            if (property_exists($value, $name) && $value->{$name} === null) {
                unset($value->{$name});
            }
        }
    }

    private static function present(mixed $value, string $key): bool
    {
        return Value::prop($value, $key) !== Undefined::Value;
    }

    private static function strings(mixed $value): bool
    {
        return is_array($value) && Value::all($value, is_string(...));
    }

    private static function badRecord(mixed $record): bool
    {
        if (! Value::truthy($record)) {
            return true;
        }
        $time = Value::prop($record, 'time');
        $date = Value::prop($record, 'date');
        $accuracy = Value::prop($record, 'accuracy');
        $mistakes = Value::prop($record, 'mistakes');
        $wordCount = Value::prop($record, 'wordCount');
        $mode = Value::prop($record, 'mode');

        return ! Js::finiteNumber($time) || $time < 0 || ! is_string($date) || ! is_finite(JsDate::parse($date)) ||
            (self::present($record, 'device') && ! Devices::isDevice($record->device)) ||
            (self::present($record, 'mode') && $mode !== 'recall' && $mode !== 'writing') ||
            (self::present($record, 'id') && ! is_string($record->id)) ||
            (self::present($record, 'accuracy') && (! Js::finiteNumber($accuracy) || $accuracy < 0 || $accuracy > 100)) ||
            (self::present($record, 'mistakes') && (! Js::integer($mistakes) || $mistakes < 0)) ||
            (self::present($record, 'wordCount') && (! Js::integer($wordCount) || $wordCount < 1)) ||
            (self::present($record, 'missed') && ! Records::missedWords($record->missed));
    }

    private static function badWord(mixed $word): bool
    {
        if (! Value::truthy($word)) {
            return true;
        }

        return ! is_string(Value::prop($word, 'display')) || ! is_string(Value::prop($word, 'input')) ||
            ! is_string(Value::prop($word, 'annotation')) || (self::present($word, 'input_type') && ! self::inputType($word->input_type));
    }

    private static function badList(mixed $list, array $ids): bool
    {
        if (! $list instanceof stdClass) {
            return true;
        }
        $id = Value::prop($list, 'id');
        $words = Value::prop($list, 'words');
        $records = Value::prop($list, 'records');

        return ! is_string($id) || $id === '' || isset($ids[$id]) || ! is_string(Value::prop($list, 'name')) ||
            ! is_string(Value::prop($list, 'createdAt')) || ! is_array($words) || ! is_array($records) ||
            (self::present($list, 'category') && ! is_string($list->category)) ||
            (self::present($list, 'tags') && ! self::strings($list->tags)) ||
            (self::present($list, 'group') && ! is_string($list->group)) ||
            (self::present($list, 'packId') && ! is_string($list->packId)) ||
            (self::present($list, 'writing') && ! self::writingMark($list->writing)) ||
            (self::present($list, 'levels') && ! self::strings($list->levels)) ||
            Value::any($words, self::badWord(...)) || Value::any($records, self::badRecord(...));
    }

    /**
     * decodeLists(JSON.stringify({ version: 1, lists: value })): the lists checked, as copies.
     *
     * @return list<stdClass>
     */
    public static function decodeLists(mixed $value): array
    {
        if ($value === Undefined::Value) {
            throw new AppError('savedDataUnreadable');
        }
        $data = Value::deepCopy($value);
        if (! is_array($data)) {
            throw new AppError('savedDataUnreadable');
        }

        foreach ($data as $list) {
            if (! $list instanceof stdClass) {
                continue;
            }
            self::dropNulls($list, self::LIST_NULLS);
            $records = Value::prop($list, 'records');
            if (is_array($records)) {
                foreach ($records as $record) {
                    if ($record instanceof stdClass) {
                        self::dropNulls($record, self::RECORD_NULLS);
                    }
                }
            }
        }
        $ids = [];
        foreach ($data as $list) {
            if (self::badList($list, $ids)) {
                throw new AppError('savedDataCorrupted');
            }

            // A list of a followed pack (with an origin) is saved without its words, which the catalog gives back.
            if ($list->words !== [] || ! is_string(Value::prop(Value::prop($list, 'origin'), 'listId'))) {
                self::validateList($list);
            } elseif (Js::trim($list->name) === '') {
                throw new AppError('listNameRequired');
            }
            $ids[$list->id] = true;
        }

        // Fields of link sharing are dropped, and an origin or weak review of another shape.
        return array_map(static function (stdClass $list): stdClass {
            $result = Value::except($list, 'shared', 'shareId', 'origin', 'weakReview');
            if (property_exists($list, 'origin') && is_string(Value::prop($list->origin, 'listId'))) {
                $result->origin = $list->origin;
            }
            if (property_exists($list, 'weakReview') && Records::weakReview($list->weakReview)) {
                $result->weakReview = $list->weakReview;
            }

            return $result;
        }, $data);
    }
}
