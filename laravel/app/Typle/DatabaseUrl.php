<?php

declare(strict_types=1);

namespace App\Typle;

use Illuminate\Support\Env;

/**
 * The database (rails/app/lib/typle/database_url.rb): the URL of DATABASE_URL (else POSTGRES_URL, as
 * src/server/database.ts reads them; in the tests TYPLE_TEST_DATABASE_URL), checked as node-postgres 8 checks it, with
 * the settings of the pool of src/server/database.ts. config/database.php reads it before the app is booted, so it
 * needs nothing of the app.
 */
final class DatabaseUrl
{
    public const ROOT_CERTIFICATES = '/etc/ssl/certs/ca-certificates.crt';

    /** @param  array<string, ?string>|callable(string): ?string  $env */
    public static function fromEnv(array|callable $env, string $appEnv): ?string
    {
        $get = is_array($env) ? static fn (string $name): ?string => $env[$name] ?? null : $env;
        if ($appEnv === 'testing') {
            $url = $get('TYPLE_TEST_DATABASE_URL');
        } else {
            $url = (string) $get('DATABASE_URL');
            if ($url === '') {
                $url = $get('POSTGRES_URL');
            }
        }

        return $url === null || $url === '' ? null : $url;
    }

    public static function configured(): bool
    {
        return config('typle.database_url') !== null;
    }

    /**
     * url with sslmode require (or prefer, or no-verify) made verify-full: node-postgres 8 checks the server's
     * certificate for those, which libpq does only for verify-full.
     */
    public static function rewrite(string $url): string
    {
        $hash = strpos($url, '#');
        $fragment = $hash === false ? '' : substr($url, $hash);
        $rest = $hash === false ? $url : substr($url, 0, $hash);
        $mark = strpos($rest, '?');
        $query = self::decodeQuery($mark === false ? '' : substr($rest, $mark + 1));
        $mode = null;
        foreach ($query as [$key, $value]) {
            if ($key === 'sslmode') {
                $mode = $value;
                break;
            }
        }
        if (! in_array($mode, ['require', 'prefer', 'verify-ca', 'no-verify'], true)) {
            return $url;
        }
        $query = array_map(static fn (array $pair): array => $pair[0] === 'sslmode' ? ['sslmode', 'verify-full'] : $pair, $query);
        if (! in_array('sslrootcert', array_column($query, 0), true)) {
            $query[] = ['sslrootcert', Env::getRepository()->get('PGSSLROOTCERT') ?: self::ROOT_CERTIFICATES];
        }
        $encoded = implode('&', array_map(static fn (array $pair): string => self::encodeComponent($pair[0]).'='.self::encodeComponent($pair[1]), $query));

        return substr($rest, 0, $mark).'?'.$encoded.$fragment;
    }

    /**
     * The connection's settings: the parts of the URL, its other parameters given to libpq as they are
     * (App\Database\PostgresConnector), each statement given up after 5 seconds, the connection after 5, TCP keep-alive,
     * and no SET statements on connecting (the client encoding and timeout go with the connection's options). Each
     * statement is sent unprepared with its parameters (PQexecParams), as node-postgres sends it, in one round trip.
     *
     * @return array<string, mixed>
     */
    public static function config(?string $url): array
    {
        $config = [
            'driver' => 'pgsql',
            'host' => null,
            'port' => null,
            'database' => null,
            'username' => null,
            'password' => null,
            'prefix' => '',
            'prefix_indexes' => true,
            'keepalives' => 1,
            'server_options' => ['statement_timeout' => 5000],
            'libpq' => ['connect_timeout' => '5', 'client_encoding' => 'UTF8'],
            'options' => class_exists(\Pdo\Pgsql::class) ? [\Pdo\Pgsql::ATTR_DISABLE_PREPARES => true] : [],
        ];
        if ($url === null) {
            return $config;
        }
        $rewritten = self::rewrite($url);
        $parts = parse_url($rewritten) ?: [];
        foreach (['host' => 'host', 'port' => 'port', 'user' => 'username', 'pass' => 'password'] as $part => $key) {
            if (isset($parts[$part])) {
                $config[$key] = $part === 'port' ? $parts[$part] : rawurldecode((string) $parts[$part]);
            }
        }
        if (isset($parts['host'])) {
            $config['host'] = trim($config['host'], '[]');
        }
        $path = ltrim($parts['path'] ?? '', '/');
        $config['database'] = $path === '' ? null : rawurldecode($path);
        foreach (self::decodeQuery($parts['query'] ?? '') as [$key, $value]) {
            $config['libpq'][$key] = $value;
        }
        return $config;
    }

    /**
     * The pairs of a query as URI.decode_www_form reads them.
     *
     * @return list<array{string, string}>
     */
    private static function decodeQuery(string $query): array
    {
        $pairs = [];
        foreach (preg_split('/[&;]/', $query, -1, PREG_SPLIT_NO_EMPTY) as $part) {
            [$key, $value] = array_pad(explode('=', $part, 2), 2, '');
            $pairs[] = [urldecode($key), urldecode($value)];
        }

        return $pairs;
    }

    /** URI.encode_www_form_component: every byte but A-Z a-z 0-9 * - . _ escaped, a space as +. */
    private static function encodeComponent(string $text): string
    {
        return str_replace('%20', '+', preg_replace_callback('/[^*\-.0-9A-Z_a-z]/', static fn (array $byte): string => sprintf('%%%02X', ord($byte[0])), $text));
    }
}
