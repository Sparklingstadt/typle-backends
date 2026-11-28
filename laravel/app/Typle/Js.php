<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * Strings and numbers as JavaScript treats them (rails/app/lib/typle/js.rb): lengths in UTF-16 units, trim() with
 * ECMAScript's white space, Number.isSafeInteger, and Go's %g of src/server/records.ts.
 */
final class Js
{
    public const SPACE = '[\t\n\x{0B}\f\r \x{A0}\x{1680}\x{2000}-\x{200A}\x{2028}\x{2029}\x{202F}\x{205F}\x{3000}\x{FEFF}]';

    public const MAX_SAFE = 9007199254740991;

    private const TRIM = '/\A'.self::SPACE.'+|'.self::SPACE.'+\z/u';

    private const TRIM_END = '/'.self::SPACE.'+\z/u';

    /** A header's value as Node reads it: each byte one character (Latin-1), whatever bytes the server received. */
    public static function header(?string $value): ?string
    {
        if ($value === null || ! preg_match('/[\x80-\xFF]/', $value)) {
            return $value;
        }

        return mb_convert_encoding($value, 'UTF-8', 'ISO-8859-1');
    }

    public static function trim(string $text): string
    {
        return preg_replace(self::TRIM, '', $text);
    }

    public static function trimEnd(string $text): string
    {
        return preg_replace(self::TRIM_END, '', $text);
    }

    /** String.prototype.toLocaleLowerCase in Node's default locale. */
    public static function lower(string $text): string
    {
        return mb_strtolower($text, 'UTF-8');
    }

    /** String.prototype.length: UTF-16 code units. */
    public static function length(string $text): int
    {
        if (! preg_match('/[\x80-\xFF]/', $text)) {
            return strlen($text);
        }

        // Every character beyond U+FFFF (two units) has one lead byte F0-F4.
        return mb_strlen($text, 'UTF-8') + preg_match_all('/[\xF0-\xF4]/', $text);
    }

    /** String.prototype.slice(0, end) without splitting a surrogate pair (what fit() of workspaceDocument.ts keeps). */
    public static function sliceTo(string $text, int $stop): string
    {
        if ($stop < 0) {
            $stop = max(self::length($text) + $stop, 0);
        }
        $units = 0;
        $kept = '';
        foreach (mb_str_split($text, 1, 'UTF-8') as $char) {
            $size = strlen($char) === 4 ? 2 : 1;
            if ($units + $size > $stop) {
                break;
            }
            $units += $size;
            $kept .= $char;
        }

        return $kept;
    }

    /** Buffer.byteLength(text): its UTF-8 bytes. */
    public static function utf8Bytesize(string $text): int
    {
        return strlen($text);
    }

    public static function finiteNumber(mixed $value): bool
    {
        return is_int($value) || (is_float($value) && is_finite($value));
    }

    public static function integer(mixed $value): bool
    {
        return is_int($value) || (is_float($value) && is_finite($value) && $value == floor($value));
    }

    public static function safeInteger(mixed $value): bool
    {
        return self::integer($value) && abs($value) <= self::MAX_SAFE;
    }

    /**
     * The shortest digits of a finite, positive magnitude and n, with magnitude = 0.digits × 10^n (ECMAScript's k, n).
     * PHP writes a float as the shortest decimal that reads back as the same double (serialize_precision -1).
     *
     * @return array{string, int}
     */
    public static function shortestDigits(float $magnitude): array
    {
        $text = var_export($magnitude, true);
        $parts = explode('e', strtolower($text));
        $mantissa = $parts[0];
        $exponent = (int) ($parts[1] ?? 0);
        $pieces = explode('.', $mantissa);
        $whole = $pieces[0];
        $fraction = $pieces[1] ?? '';
        $digits = $whole.$fraction;
        $n = strlen($whole) + $exponent;
        while (strlen($digits) > 1 && $digits[0] === '0') {
            $digits = substr($digits, 1);
            $n--;
        }
        $digits = rtrim($digits, '0');
        if ($digits === '') {
            $digits = '0';
        }

        return [$digits, $n];
    }

    /** Number.prototype.toString of a finite number (JSON.stringify writes numbers so; -0 is "0"). */
    public static function number(int|float $value): string
    {
        if (is_int($value)) {
            return (string) $value;
        }
        if ($value == 0.0) {
            return '0';
        }
        if ($value < 0) {
            return '-'.self::number(-$value);
        }
        [$digits, $n] = self::shortestDigits($value);
        $k = strlen($digits);
        if ($n >= $k && $n <= 21) {
            return $digits.str_repeat('0', $n - $k);
        }
        if ($n >= 1 && $n <= 21) {
            return substr($digits, 0, $n).'.'.substr($digits, $n);
        }
        if ($n >= -5 && $n <= 0) {
            return '0.'.str_repeat('0', -$n).$digits;
        }
        $e = $n - 1;
        $mantissa = $k === 1 ? $digits : $digits[0].'.'.substr($digits, 1);

        return $mantissa.'e'.($e < 0 ? '-' : '+').abs($e);
    }

    /** value as Go's strconv.FormatFloat(value, 'g', -1, 64) writes it (goFloat of src/server/records.ts). */
    public static function goFloat(int|float $value): string
    {
        $value = (float) $value;
        if ($value == 0.0) {
            return (fdiv(1.0, $value) < 0) ? '-0' : '0';
        }
        if ($value < 0) {
            return '-'.self::goFloat(-$value);
        }
        [$digits, $n] = self::shortestDigits($value);
        $exponent = $n - 1;
        if ($exponent < -4 || $exponent >= 6) {
            $mantissa = strlen($digits) === 1 ? $digits : $digits[0].'.'.substr($digits, 1);

            return $mantissa.'e'.($exponent < 0 ? '-' : '+').str_pad((string) abs($exponent), 2, '0', STR_PAD_LEFT);
        }

        return self::number($value);
    }
}
