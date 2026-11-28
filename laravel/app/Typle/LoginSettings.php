<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * The login settings as typle-r reads them (rails/app/lib/typle/login_settings.rb): AUTH_URL as readLoginSettings of
 * src/server/settings.ts (the app's origin), BETTER_AUTH_SECRET as betterAuthSettings of src/server/authUser.ts
 * (trimmed) and DEMO_LOGIN as demoConfigured.
 */
final readonly class LoginSettings
{
    public function __construct(
        public string $origin,
        public bool $secure,
        public string $secret,
        public bool $demoLogin,
    ) {}

    /**
     * null when AUTH_URL is not an http(s) URL with a host (readLoginSettings is null: 503 authUnavailable). The secret
     * may be empty: Better Auth then fails to start, which the routes that need the login answer with 503
     * databaseUnavailable.
     *
     * @param  array{AUTH_URL?: ?string, BETTER_AUTH_SECRET?: ?string, DEMO_LOGIN?: ?string}  $env
     */
    public static function read(array $env): ?self
    {
        $origin = self::originOf((string) ($env['AUTH_URL'] ?? ''));
        if ($origin === null) {
            return null;
        }

        return new self($origin[0], $origin[1], Js::trim((string) ($env['BETTER_AUTH_SECRET'] ?? '')), ($env['DEMO_LOGIN'] ?? null) === '1');
    }

    /** The settings of config/typle.php, read once for each set of values (as the Rails backend reads them). */
    public static function current(): ?self
    {
        static $cache = [];
        $env = [
            'AUTH_URL' => config('typle.auth_url'),
            'BETTER_AUTH_SECRET' => config('typle.better_auth_secret'),
            'DEMO_LOGIN' => config('typle.demo_login'),
        ];
        $key = serialize($env);
        if (! array_key_exists($key, $cache)) {
            $cache[$key] = self::read($env);
        }

        return $cache[$key];
    }

    /**
     * The origin of an http or https URL as new URL(text).origin gives it, and whether it is https.
     *
     * @return array{string, bool}|null
     */
    public static function originOf(string $text): ?array
    {
        $text = preg_replace('/\A[\x00-\x20]+|[\x00-\x20]+\z/', '', $text);
        if (preg_match('/[\x00-\x20\x7F-\xFF]/', $text)) {
            return null;
        }
        $parts = parse_url($text);
        if ($parts === false) {
            return null;
        }
        $scheme = strtolower($parts['scheme'] ?? '');
        $host = strtolower($parts['host'] ?? '');
        if (! in_array($scheme, ['http', 'https'], true) || $host === '') {
            return null;
        }
        $default = $scheme === 'https' ? 443 : 80;
        $port = ! isset($parts['port']) || $parts['port'] === $default ? '' : ':'.$parts['port'];

        return ["{$scheme}://{$host}{$port}", $scheme === 'https'];
    }

    /** The login cookie's name: __Secure- over HTTPS. */
    public function cookieName(): string
    {
        return Sessions::cookieName($this->secure);
    }
}
