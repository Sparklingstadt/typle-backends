<?php

declare(strict_types=1);

namespace App\Typle;

use InvalidArgumentException;

/**
 * Dates as V8's Date.parse reads them and Date.prototype.toISOString writes them (rails/app/lib/typle/js_date.rb).
 * Times are milliseconds since the epoch, as floats (NAN when invalid). Without an offset a date and time is local time,
 * which is UTC where the servers run. Days are counted in the proleptic Gregorian calendar, as ECMAScript counts them.
 */
final class JsDate
{
    public const MAX_TIME = 8.64e15;

    public const DAY_MS = 86400000;

    private const ISO = '/\A([+-]\d{6}|\d{4})(?:-(\d{2})(?:-(\d{2}))?)?(?:T(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d+))?)?)?(Z|[+-]\d{2}:\d{2})?\z/';

    private const NUMERIC = '/\A\s*(\d{1,6})[-\/](\d{1,2})[-\/](\d{1,2})(?:[T ]+(\d{1,2}):(\d{2})(?::(\d{2})(?:\.(\d+))?)?)?\s*(Z|UTC|GMT|(?:(?:UTC|GMT)?[+-]\d{2}:?\d{2}))?\s*\z/i';

    private const WORDED = '/\A\s*(?:[A-Za-z]+,?\s+)?(?:(\d{1,2})\s+([A-Za-z]+)|([A-Za-z]+)\s+(\d{1,2}),?)\s+(\d{1,6})(?:\s+(\d{1,2}):(\d{2})(?::(\d{2}))?)?\s*((?:UTC|GMT|Z)?(?:[+-]\d{2}:?\d{2})?)\s*(?:\([^)]*\))?\s*\z/i';

    private const MONTHS = ['jan', 'feb', 'mar', 'apr', 'may', 'jun', 'jul', 'aug', 'sep', 'oct', 'nov', 'dec'];

    private const OFFSET = '/([+-])(\d{2}):?(\d{2})\z/';

    /** Date.parse(text): milliseconds since the epoch, or NAN. */
    public static function parse(string $text): float
    {
        if (preg_match(self::ISO, $text, $match, PREG_UNMATCHED_AS_NULL)) {
            return self::parseIso($match);
        }

        return self::parseLegacy($text);
    }

    /** Date.prototype.toISOString of a valid time. */
    public static function toIso(float $time): string
    {
        if (is_nan($time)) {
            throw new InvalidArgumentException('Invalid time value');
        }
        $t = (int) $time;
        $days = self::floorDiv($t, self::DAY_MS);
        $inDay = $t - $days * self::DAY_MS;
        [$year, $month, $day] = self::civilFromDays($days);
        $yearText = $year >= 0 && $year <= 9999
            ? str_pad((string) $year, 4, '0', STR_PAD_LEFT)
            : ($year < 0 ? '-' : '+').str_pad((string) abs($year), 6, '0', STR_PAD_LEFT);

        return sprintf('%s-%02d-%02dT%02d:%02d:%02d.%03dZ', $yearText, $month, $day,
            intdiv($inDay, 3600000), intdiv($inDay, 60000) % 60, intdiv($inDay, 1000) % 60, $inDay % 1000);
    }

    /** getUTCFullYear of a time, NAN for NAN. */
    public static function utcYear(float $time): float
    {
        if (is_nan($time)) {
            return NAN;
        }

        return (float) self::civilFromDays(self::floorDiv((int) $time, self::DAY_MS))[0];
    }

    /** ECMAScript's MakeDay (with month overflow) and MakeDate, in UTC. */
    public static function makeTime(int $year, int $month, int $day, int $hour, int $minute, int $second, int $ms): float
    {
        $y = $year + self::floorDiv($month, 12);
        if (abs($y) > 400000) {
            return NAN;
        }
        $days = self::daysFromCivil($y, $month - self::floorDiv($month, 12) * 12 + 1, 1) + $day - 1;

        return $days * (float) self::DAY_MS + ((($hour * 60) + $minute) * 60 + $second) * 1000.0 + $ms;
    }

    private static function clip(float $time): float
    {
        return is_nan($time) || abs($time) > self::MAX_TIME ? NAN : $time;
    }

    private static function millis(?string $text): int
    {
        return $text === null ? 0 : (int) substr($text.'00', 0, 3);
    }

    private static function parseIso(array $match): float
    {
        [, $yearText, $month, $day, $hour, $minute, $second, $fraction, $zone] = array_pad($match, 9, null);
        if ($yearText === '-000000') {
            return NAN;
        }
        $month = $month !== null ? (int) $month : 1;
        $day = $day !== null ? (int) $day : 1;
        $hasTime = $hour !== null;
        $hour = (int) $hour;
        $minute = (int) $minute;
        $second = (int) $second;
        $ms = self::millis($fraction);
        if ($month < 1 || $month > 12 || $day < 1 || $day > 31 || $minute > 59 || $second > 59) {
            return NAN;
        }
        if ($hour > 24 || ($hour === 24 && ($minute !== 0 || $second !== 0 || $ms !== 0))) {
            return NAN;
        }
        if (! $hasTime && $zone !== null) {
            return NAN;
        }
        $time = self::makeTime((int) $yearText, $month - 1, $day, $hour, $minute, $second, $ms);
        if ($zone !== null && $zone !== 'Z') {
            $sign = $zone[0] === '-' ? -1 : 1;
            $offHour = (int) substr($zone, 1, 2);
            $offMinute = (int) substr($zone, 4, 2);
            if ($offHour > 23 || $offMinute > 59) {
                return NAN;
            }
            $time -= $sign * (($offHour * 60) + $offMinute) * 60000.0;
        }

        return self::clip($time);
    }

    private static function parseLegacy(string $text): float
    {
        if (preg_match(self::NUMERIC, $text, $m, PREG_UNMATCHED_AS_NULL)) {
            $m = array_pad($m, 9, null);

            return self::legacyTime((int) $m[1], (int) $m[2], (int) $m[3], $m[4], $m[5], $m[6], self::millis($m[7]), (string) $m[8]);
        }
        if (preg_match(self::WORDED, $text, $m, PREG_UNMATCHED_AS_NULL)) {
            $m = array_pad($m, 10, null);
            [$dayText, $monthText] = $m[1] !== null ? [$m[1], $m[2]] : [$m[4], $m[3]];
            $month = array_search(strtolower(substr($monthText, 0, 3)), self::MONTHS, true);
            if ($month === false || strlen($monthText) < 3) {
                return NAN;
            }

            return self::legacyTime((int) $m[5], $month + 1, (int) $dayText, $m[6], $m[7], $m[8], 0, (string) $m[9]);
        }

        return NAN;
    }

    private static function legacyTime(int $year, int $month, int $day, ?string $hour, ?string $minute, ?string $second, int $ms, string $zone): float
    {
        if ($month < 1 || $month > 12 || $day < 1 || $day > 31) {
            return NAN;
        }
        $h = (int) $hour;
        $mi = (int) $minute;
        $s = (int) $second;
        if ($h > 24 || $mi > 59 || $s > 59) {
            return NAN;
        }
        $time = self::makeTime($year, $month - 1, $day, $h, $mi, $s, $ms);
        if (preg_match(self::OFFSET, $zone, $offset)) {
            $sign = $offset[1] === '-' ? -1 : 1;
            $time -= $sign * (((int) $offset[2] * 60) + (int) $offset[3]) * 60000.0;
        }

        return self::clip($time);
    }

    private static function floorDiv(int $a, int $b): int
    {
        $q = intdiv($a, $b);

        return ($a % $b !== 0 && (($a < 0) !== ($b < 0))) ? $q - 1 : $q;
    }

    /** The days since 1970-01-01 of a date of the proleptic Gregorian calendar. */
    private static function daysFromCivil(int $y, int $m, int $d): int
    {
        $y -= $m <= 2 ? 1 : 0;
        $era = self::floorDiv($y, 400);
        $yoe = $y - $era * 400;
        $doy = intdiv(153 * (($m + 9) % 12) + 2, 5) + $d - 1;
        $doe = $yoe * 365 + intdiv($yoe, 4) - intdiv($yoe, 100) + $doy;

        return $era * 146097 + $doe - 719468;
    }

    /**
     * The date of a number of days since 1970-01-01, in the proleptic Gregorian calendar.
     *
     * @return array{int, int, int}
     */
    private static function civilFromDays(int $z): array
    {
        $z += 719468;
        $era = self::floorDiv($z, 146097);
        $doe = $z - $era * 146097;
        $yoe = intdiv($doe - intdiv($doe, 1460) + intdiv($doe, 36524) - intdiv($doe, 146096), 365);
        $doy = $doe - (365 * $yoe + intdiv($yoe, 4) - intdiv($yoe, 100));
        $mp = intdiv(5 * $doy + 2, 153);
        $d = $doy - intdiv(153 * $mp + 2, 5) + 1;
        $m = $mp < 10 ? $mp + 3 : $mp - 9;

        return [$yoe + $era * 400 + ($m <= 2 ? 1 : 0), $m, $d];
    }
}
