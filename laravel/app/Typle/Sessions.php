<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * The login cookie that Better Auth sets in typle-r (src/server/betterAuth.ts): the session token, signed with
 * BETTER_AUTH_SECRET as better-call's signCookieValue signs it (rails/app/lib/typle/sessions.rb). The cookie only names a
 * session; who is signed in, and until when, is the session's row in typle_auth_sessions (AuthSession::userOf). Only
 * reading is served here: typle-r signs the users in, and sign is kept for the tests. The apps' bearer tokens are gone
 * from typle-r and not read.
 *
 * The cookie is read from the Cookie header as it came, not through Laravel's cookies (which EncryptCookies decrypts and
 * Symfony decodes).
 */
final class Sessions
{
    /**
     * An HMAC-SHA256 in standard base64 with its padding: 44 characters, the only length better-call's getSignedCookie
     * accepts, ending in "=". Unused bits are not checked (Go's base64.StdEncoding does not either).
     */
    private const SIGNATURE = '~\A[A-Za-z0-9+/]{42}(?:[A-Za-z0-9+/]=|==)\z~';

    /**
     * The name of Better Auth's session cookie: better-auth.session_token, with the __Secure- prefix when AUTH_URL is
     * HTTPS (createCookieGetter in better-auth's cookies).
     */
    public static function cookieName(bool $secure): string
    {
        return $secure ? '__Secure-better-auth.session_token' : 'better-auth.session_token';
    }

    /**
     * The session token of a session cookie's value as the Cookie header carries it, as better-call's getSignedCookie
     * reads it: the value is URL-decoded (kept as it is when it cannot be, as tryDecode does), then split at its last "."
     * into the token and its signature, which must be the HMAC-SHA256 of the token under secret in standard base64. null
     * for any other value, and for an empty secret.
     */
    public static function token(string $value, ?string $secret): ?string
    {
        if ($secret === null || $secret === '') {
            return null;
        }
        $value = self::decodeUriComponent($value);
        $at = strrpos($value, '.');
        if ($at === false || $at < 1) {
            return null;
        }
        $token = substr($value, 0, $at);
        $signature = substr($value, $at + 1);
        if (! preg_match(self::SIGNATURE, $signature)) {
            return null;
        }
        $sent = base64_decode($signature);
        $want = self::mac($token, $secret);

        return $sent !== false && strlen($sent) === strlen($want) && hash_equals($want, $sent) ? $token : null;
    }

    /**
     * The cookie value Better Auth sets for token (signCookieValue): the token, ".", its signature, URL-encoded as
     * encodeURIComponent encodes it.
     */
    public static function sign(string $token, string $secret): string
    {
        return self::encodeUriComponent($token.'.'.base64_encode(self::mac($token, $secret)));
    }

    private static function mac(string $token, string $secret): string
    {
        return hash_hmac('sha256', $token, $secret, true);
    }

    /**
     * decodeURIComponent, or text as it is when that throws (a % not followed by two hex digits, or escapes that are not
     * UTF-8).
     */
    public static function decodeUriComponent(string $text): string
    {
        if (! str_contains($text, '%') || preg_match('/%(?![0-9A-Fa-f]{2})/', $text)) {
            return $text;
        }
        $decoded = rawurldecode($text);

        return mb_check_encoding($decoded, 'UTF-8') ? $decoded : $text;
    }

    /** encodeURIComponent: every byte of the UTF-8 text but A-Z a-z 0-9 - _ . ! ~ * ' ( ) escaped. */
    public static function encodeUriComponent(string $text): string
    {
        return preg_replace_callback("/[^A-Za-z0-9\\-_.!~*'()]/", static fn (array $byte): string => sprintf('%%%02X', ord($byte[0])), $text);
    }

    /**
     * The value of the first cookie named name in a Cookie header, as better-call's parseCookies reads it: the first pair
     * with that name wins, and a value that starts with a double quote loses its first and last character. The value is
     * not decoded here (token does).
     */
    public static function cookieValue(?string $header, string $name): ?string
    {
        foreach (explode(';', (string) Js::header($header ?? '')) as $part) {
            $at = strpos($part, '=');
            if ($at === false || Js::trim(substr($part, 0, $at)) !== $name) {
                continue;
            }
            $value = Js::trim(substr($part, $at + 1));
            if (! str_starts_with($value, '"')) {
                return $value;
            }
            $length = mb_strlen($value, 'UTF-8');

            return mb_substr($value, 1, max(1, $length - 1) - 1, 'UTF-8');
        }

        return null;
    }
}
