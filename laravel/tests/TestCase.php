<?php

namespace Tests;

use App\Typle\JsonValue;
use Illuminate\Foundation\Testing\TestCase as BaseTestCase;
use Illuminate\Testing\TestResponse;
use stdClass;

/**
 * The app booted for each test (rails/test/test_helper.rb). The DB tests check the rows other connections leave (two
 * saves at once), so no test runs in a transaction; they add rows of random owners and delete them afterwards.
 */
abstract class TestCase extends BaseTestCase
{
    /**
     * A request with the headers given as they are (Origin, Content-Type, Cookie...) and the raw body.
     *
     * @param  array<string, ?string>  $headers
     */
    protected function send(string $method, string $uri, array $headers = [], ?string $content = null): TestResponse
    {
        $headers = array_filter($headers, static fn ($value) => $value !== null);

        return $this->call($method, $uri, [], [], [], $this->transformHeadersToServerVars($headers), $content);
    }

    protected function assertError(TestResponse $response, int $status, string $key): void
    {
        $this->assertSame($status, $response->getStatusCode(), (string) $response->getContent());
        $this->assertSame("{\"error\":\"{$key}\"}\n", $response->getContent());
    }

    /** A JSON value as a string to compare, its objects' keys sorted: compared as Ruby compares Hashes (whatever the keys' order). */
    protected static function comparable(mixed $value): string
    {
        return json_encode(self::sorted(JsonValue::parse(JsonValue::generate($value))), JSON_UNESCAPED_UNICODE);
    }

    private static function sorted(mixed $value): mixed
    {
        if ($value instanceof stdClass) {
            $pairs = [];
            foreach ($value as $key => $item) {
                $pairs[(string) $key] = self::sorted($item);
            }
            ksort($pairs, SORT_STRING);

            return ['{}' => $pairs];
        }

        return is_array($value) ? array_map(self::sorted(...), $value) : $value;
    }
}
