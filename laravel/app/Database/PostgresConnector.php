<?php

declare(strict_types=1);

namespace App\Database;

use Illuminate\Database\Connectors\PostgresConnector as BaseConnector;

/**
 * Laravel's connector for PostgreSQL, which also gives libpq the parameters of the URL it knows nothing of (sslmode,
 * sslrootcert, channel_binding... App\Typle\DatabaseUrl::config puts them under "libpq"), as node-postgres and the
 * Rails backend pass the whole URL on.
 */
class PostgresConnector extends BaseConnector
{
    protected function getDsn(array $config)
    {
        $dsn = parent::getDsn($config);
        foreach ($config['libpq'] ?? [] as $key => $value) {
            $dsn .= ';'.$key."='".str_replace(['\\', "'"], ['\\\\', "\\'"], (string) $value)."'";
        }

        return $dsn;
    }
}
