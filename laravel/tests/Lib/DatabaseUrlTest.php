<?php

namespace Tests\Lib;

use App\Typle\DatabaseUrl;
use Tests\TestCase;

class DatabaseUrlTest extends TestCase
{
    public function test_a_neon_url_is_checked_as_node_postgres_checks_it(): void
    {
        $url = DatabaseUrl::rewrite('postgresql://user:p%40ss@ep-x.ap-southeast-1.aws.neon.tech/neondb?sslmode=require&channel_binding=require');
        parse_str((string) parse_url($url, PHP_URL_QUERY), $query);
        $this->assertSame('verify-full', $query['sslmode']);
        $this->assertSame('require', $query['channel_binding']);
        $this->assertSame('/etc/ssl/certs/ca-certificates.crt', $query['sslrootcert']);
        $this->assertSame('p%40ss', parse_url($url, PHP_URL_PASS));

        $config = DatabaseUrl::config('postgresql://user:p%40ss@ep-x.ap-southeast-1.aws.neon.tech/neondb?sslmode=require&channel_binding=require');
        $this->assertSame('p@ss', $config['password']);
        $this->assertSame('neondb', $config['database']);
        $this->assertSame(['connect_timeout' => '5', 'client_encoding' => 'UTF8', 'sslmode' => 'verify-full', 'channel_binding' => 'require',
            'sslrootcert' => '/etc/ssl/certs/ca-certificates.crt'], $config['libpq']);
    }

    public function test_a_local_url_is_left_as_it_is_and_the_connection_is_set_up_as_next_js_sets_up_its_pool(): void
    {
        $this->assertSame('postgres://typle:x@127.0.0.1:5433/typle', DatabaseUrl::rewrite('postgres://typle:x@127.0.0.1:5433/typle'));
        $config = DatabaseUrl::config('postgres://typle:x@127.0.0.1:5433/typle');
        $this->assertSame(['127.0.0.1', 5433, 'typle', 'typle', 'x'], [$config['host'], $config['port'], $config['database'], $config['username'], $config['password']]);
        $this->assertSame('5', $config['libpq']['connect_timeout']);
        $this->assertSame(1, $config['keepalives']);
        $this->assertSame(['statement_timeout' => 5000], $config['server_options']);
        $this->assertSame([\Pdo\Pgsql::ATTR_DISABLE_PREPARES => true], $config['options']);
    }

    public function test_the_url_is_database_url_else_postgres_url_and_typle_test_database_url_in_the_tests(): void
    {
        $this->assertSame('postgres://a', DatabaseUrl::fromEnv(['DATABASE_URL' => 'postgres://a', 'POSTGRES_URL' => 'postgres://b'], 'production'));
        $this->assertSame('postgres://b', DatabaseUrl::fromEnv(['DATABASE_URL' => '', 'POSTGRES_URL' => 'postgres://b'], 'production'));
        $this->assertSame('postgres://t', DatabaseUrl::fromEnv(['DATABASE_URL' => 'postgres://a', 'TYPLE_TEST_DATABASE_URL' => 'postgres://t'], 'testing'));
        $this->assertNull(DatabaseUrl::fromEnv([], 'production'));
    }
}
