<?php

namespace Tests\Integration;

use App\Typle\JsonValue;
use App\Typle\Report;
use App\Typle\Saved;
use App\Typle\Session;
use App\Typle\Sessions;
use App\Typle\SessionUsers;
use App\Typle\WorkspaceChange;
use App\Typle\WorkspaceStore;
use DateTimeInterface;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Date;
use RuntimeException;
use Tests\TestCase;
use Throwable;

/**
 * The login as server/internal/web/sessions_test.go checks it, without a database (rails/test/integration/
 * sessions_test.rb): the sessions and workspaces are stood in for (a token "session-of:<owner>" is a session of owner),
 * so only the reading of the cookie and the answers are checked here. WorkspaceTest reads real session rows when
 * TYPLE_TEST_DATABASE_URL is set.
 */
class SessionsTest extends TestCase
{
    private const SECRET = 'test secret';

    private const ORIGIN = 'http://127.0.0.1:3101';

    private const OWNER = 'github:1';

    /** @var list<string> */
    private array $reports = [];

    /** @var list<DateTimeInterface|string> */
    private array $askedAt = [];

    protected function setUp(): void
    {
        parent::setUp();
        config(['typle.compare' => true, 'typle.auth_url' => self::ORIGIN, 'typle.better_auth_secret' => self::SECRET, 'typle.demo_login' => null]);
    }

    private static function tokenOf(string $owner): string
    {
        return "session-of:{$owner}";
    }

    /** Better Auth's session cookie for a session of owner, as a Cookie header. */
    private static function cookie(string $owner, string $secret = self::SECRET, bool $secure = false): string
    {
        return Sessions::cookieName($secure).'='.Sessions::sign(self::tokenOf($owner), $secret);
    }

    /**
     * A database said to be there, the sessions stood in for and an empty workspace that saves anything.
     *
     * @param  (callable(string, DateTimeInterface|string): ?Session)|null  $userOf
     */
    private function withFakes(?callable $userOf = null): void
    {
        $test = $this;
        $userOf ??= function (string $token, DateTimeInterface|string $now): ?Session {
            $this->askedAt[] = $now;
            $owner = str_starts_with($token, 'session-of:') ? substr($token, strlen('session-of:')) : null;

            return $owner === null ? null : new Session($owner, '', '');
        };
        config(['typle.database_url' => 'postgres://stand-in']);
        $this->app->instance(SessionUsers::class, new class($userOf) extends SessionUsers
        {
            public function __construct(private $userOf) {}

            public function userOf(string $token, DateTimeInterface|string $now): ?Session
            {
                return ($this->userOf)($token, $now);
            }
        });
        $this->app->instance(WorkspaceStore::class, new class extends WorkspaceStore
        {
            public function read(string $owner): ?Saved
            {
                return null;
            }

            public function write(string $owner, int $revision, WorkspaceChange $change): ?Saved
            {
                return new Saved([], null, [], null, 2);
            }
        });
        $this->app->instance(Report::class, new class($test) extends Report
        {
            public function __construct(private SessionsTest $test) {}

            public function failure(Request $request, Throwable $error, string $operation): void
            {
                $this->test->reported($operation);
            }
        });
    }

    public function reported(string $operation): void
    {
        $this->reports[] = $operation;
    }

    private function read(array $headers = [])
    {
        return $this->send('GET', '/api/workspace', $headers);
    }

    private function save(array $headers = [])
    {
        return $this->send('PUT', '/api/workspace',
            array_merge(['Origin' => self::ORIGIN, 'Content-Type' => 'application/json', 'Cookie' => self::cookie(self::OWNER)], $headers),
            '{"revision":1,"accountId":"'.self::OWNER.'","devices":[]}');
    }

    public function test_a_better_auth_session_opens_the_accounts_data_and_no_answer_sets_a_cookie(): void
    {
        $this->withFakes();
        $response = $this->read(['Cookie' => self::cookie(self::OWNER)]);
        $this->assertSame(200, $response->getStatusCode(), $response->getContent());
        $this->assertSame(self::OWNER, JsonValue::parse($response->getContent())->accountId);
        $response = $this->save();
        $this->assertSame(200, $response->getStatusCode(), $response->getContent());
        $response = $this->read();
        $this->assertError($response, 401, 'loginRequired');
        $this->assertNull($response->headers->get('Set-Cookie'));
    }

    public function test_over_https_the_cookie_is_the_secure_one_and_the_plain_one_is_no_ones(): void
    {
        config(['typle.auth_url' => 'https://typle.example']);
        $this->withFakes();
        $response = $this->read(['Cookie' => self::cookie(self::OWNER, secure: true)]);
        $this->assertSame(200, $response->getStatusCode(), $response->getContent());
        $this->assertError($this->read(['Cookie' => self::cookie(self::OWNER, secure: false)]), 401, 'loginRequired');
    }

    public function test_a_cookie_that_is_not_a_sessions_is_login_required(): void
    {
        $this->withFakes();
        $tampered = preg_replace_callback('/.%3D\z/', static fn ($tail) => ($tail[0][0] === 'A' ? 'B' : 'A').'%3D', self::cookie(self::OWNER));
        foreach ([
            'Auth.js cookie' => 'authjs.session-token=x',
            'tampered' => $tampered,
            'other secret' => self::cookie(self::OWNER, 'another secret'),
            '__Secure- over http' => self::cookie(self::OWNER, secure: true),
            'no such session' => 'better-auth.session_token='.Sessions::sign('gone', self::SECRET),
        ] as $name => $header) {
            $response = $this->read(['Cookie' => $header]);
            $this->assertSame(401, $response->getStatusCode(), $name);
            $this->assertNull($response->headers->get('Set-Cookie'), $name);
        }
    }

    /**
     * The iPad and Mac apps' bearer tokens are gone from typle-r: an Authorization header neither opens the data nor
     * stands in for the cookie's Origin.
     */
    public function test_an_authorization_header_is_ignored(): void
    {
        $this->withFakes();
        $this->assertError($this->read(['Authorization' => 'Bearer x']), 401, 'loginRequired');
        $response = $this->read(['Authorization' => 'Bearer x', 'Cookie' => self::cookie(self::OWNER)]);
        $this->assertSame(200, $response->getStatusCode(), $response->getContent());
        $this->assertError($this->save(['Origin' => null, 'Authorization' => 'Bearer x']), 403, 'forbiddenRequest');
    }

    public function test_when_the_sessions_cannot_be_read_the_data_is_not_served(): void
    {
        $this->withFakes(static fn () => throw new RuntimeException('down'));
        $this->assertError($this->read(['Cookie' => self::cookie(self::OWNER)]), 503, 'databaseUnavailable');
        $this->assertSame(['session.check'], $this->reports);
    }

    /** Without BETTER_AUTH_SECRET Better Auth does not start in typle-r, and reading the session throws. */
    public function test_without_better_auth_secret_every_request_that_needs_the_login_is_503_database_unavailable(): void
    {
        config(['typle.better_auth_secret' => ' ']);
        $this->withFakes();
        $this->assertError($this->read(['Cookie' => self::cookie(self::OWNER)]), 503, 'databaseUnavailable');
        $this->assertError($this->read(), 503, 'databaseUnavailable');
        $this->assertSame(['session.check', 'session.check'], $this->reports);
    }

    public function test_the_shared_demo_accounts_sessions_work_only_while_demo_login_is_1(): void
    {
        $this->withFakes();
        $this->assertError($this->read(['Cookie' => self::cookie('demo:shared')]), 401, 'loginRequired');
        config(['typle.demo_login' => '1']);
        $response = $this->read(['Cookie' => self::cookie('demo:shared')]);
        $this->assertSame(200, $response->getStatusCode(), $response->getContent());
    }

    public function test_the_cookie_is_the_first_of_its_name_a_leading_quote_dropped_with_the_last_character(): void
    {
        $name = Sessions::cookieName(false);
        $good = substr(self::cookie(self::OWNER), strlen("{$name}="));
        $this->withFakes();
        foreach ([
            "{$name}={$good}" => 200,
            "a=b; {$name}=\"{$good}\"" => 200,
            " {$name} = {$good} ;x=y" => 200,
            "{$name}=bad; {$name}={$good}" => 401,
            "__Secure-{$name}={$good}" => 401,
            "better-auth.session_data={$good}" => 401,
            '' => 401,
        ] as $header => $status) {
            $this->assertSame($status, $this->read(['Cookie' => (string) $header])->getStatusCode(), 'Cookie '.var_export($header, true));
        }
    }

    public function test_the_session_is_looked_up_at_the_time_of_the_request(): void
    {
        $before = Date::now();
        $this->withFakes();
        $this->assertSame(200, $this->read(['Cookie' => self::cookie(self::OWNER)])->getStatusCode());
        $this->assertCount(1, $this->askedAt);
        $this->assertGreaterThanOrEqual($before, $this->askedAt[0]);
        $this->assertLessThanOrEqual(Date::now(), $this->askedAt[0]);
    }
}
