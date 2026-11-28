<?php

namespace Tests\Integration;

use App\Models\AuthSession;
use App\Models\AuthUser;
use App\Models\Workspace;
use App\Typle\DatabaseUrl;
use App\Typle\Interests;
use App\Typle\JsonValue;
use App\Typle\Saved;
use App\Typle\Session;
use App\Typle\Sessions;
use App\Typle\Value;
use App\Typle\WorkspaceChange;
use App\Typle\WorkspaceStore;
use Illuminate\Database\Events\QueryExecuted;
use Illuminate\Database\Query\Expression;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Date;
use Illuminate\Testing\TestResponse;
use RuntimeException;
use stdClass;
use Symfony\Component\Process\Process;
use Tests\Fixtures;
use Tests\Support\Saves;
use Tests\TestCase;

/**
 * The endpoints as rails/test/integration/workspace_test.rb checks them: without a database first, then the scenarios the
 * Go backend ran against a real database (TYPLE_TEST_DATABASE_URL; skipped when it is not set).
 */
class WorkspaceTest extends TestCase
{
    private const SECRET = 'typle-laravel-test-secret';

    private const ORIGIN = 'http://127.0.0.1:3101';

    private const AT = '2026-09-07T00:00:00.000Z';

    /** @var list<string> The owners this test made, whose rows are deleted after it. */
    private array $owners = [];

    protected function setUp(): void
    {
        parent::setUp();
        config(['typle.compare' => true, 'typle.better_auth_secret' => self::SECRET, 'typle.auth_url' => self::ORIGIN, 'typle.demo_login' => null]);
    }

    protected function tearDown(): void
    {
        if ($this->owners !== [] && DatabaseUrl::configured()) {
            foreach (['typle_records', 'typle_workspaces'] as $table) {
                DB::table($table)->whereIn('owner_id', $this->owners)->delete();
            }
            // The session rows go with their users (ON DELETE CASCADE).
            AuthUser::query()->whereIn('id', $this->owners)->delete();
        }
        parent::tearDown();
    }

    private function database(): void
    {
        if (! DatabaseUrl::configured()) {
            $this->markTestSkipped('TYPLE_TEST_DATABASE_URL is not set');
        }
    }

    /** A new random account with its Better Auth user and a session of a day, whose token the cookie of headersFor names. */
    private function newOwner(): string
    {
        $owner = 'github:'.random_int(1_000_000_000, 1_999_999_999);
        $this->owners[] = $owner;
        AuthUser::query()->insert(['id' => $owner, 'name' => 'Test', 'email' => "{$owner}@users.invalid", 'email_verified' => new Expression('true')]);
        AuthSession::query()->insert(['id' => $owner, 'token' => self::tokenOf($owner), 'user_id' => $owner,
            'expires_at' => Date::now()->addDay()->format('Y-m-d\TH:i:s.uP')]);

        return $owner;
    }

    private static function tokenOf(string $owner): string
    {
        return "session-of:{$owner}";
    }

    private static function headersFor(?string $owner, array $extra = []): array
    {
        $headers = ['Origin' => self::ORIGIN, 'Content-Type' => 'application/json'];
        if ($owner !== null) {
            $headers['Cookie'] = 'better-auth.session_token='.Sessions::sign(self::tokenOf($owner), self::SECRET);
        }

        return array_merge($headers, $extra);
    }

    private static function library(string $name): string
    {
        return '"packs":[{"id":"p","name":"P","createdAt":"'.self::AT.'"}],"lists":[{"id":"l","name":"'.$name.'","packId":"p","words":[{"display":"a","input":"a","annotation":""}],"records":[],"createdAt":"'.self::AT.'"}]';
    }

    private static function answer(TestResponse $response): mixed
    {
        return JsonValue::parse($response->getContent());
    }


    private function insertRow(string $owner, stdClass $row): void
    {
        $json = static fn ($value) => $value === null ? null : JsonValue::generate($value);
        Workspace::query()->insert(['owner_id' => $owner, 'lists' => $json($row->lists), 'packs' => $json($row->packs), 'devices' => $json($row->devices),
            'interests' => $json($row->interests), 'revision' => $row->revision, 'records_split' => new Expression($row->split ? 'true' : 'false')]);
    }

    private function state(string $owner): stdClass
    {
        $row = DB::selectOne(<<<'SQL'
            SELECT COALESCE((SELECT jsonb_build_object('lists', lists, 'packs', packs, 'devices', devices, 'interests', interests,
              'revision', revision, 'split', records_split) FROM typle_workspaces WHERE owner_id = ?), 'null'::jsonb)::text AS value
            SQL, [$owner])->value;
        $records = DB::selectOne(<<<'SQL'
            SELECT COALESCE(jsonb_agg(jsonb_build_array(list_id, record_id, position, (extract(epoch FROM at) * 1000)::bigint, data)
              ORDER BY list_id, position), '[]'::jsonb)::text AS value FROM typle_records WHERE owner_id = ?
            SQL, [$owner])->value;

        return Value::object(['row' => JsonValue::parse($row), 'records' => JsonValue::parse($records)]);
    }

    // Without a database

    public function test_without_a_database_the_health_is_503_and_a_write_is_refused_before_it_is_opened(): void
    {
        config(['typle.database_url' => null]);
        $this->assertError($this->send('GET', '/api/health'), 503, 'databaseUnavailable');
        $response = $this->send('HEAD', '/api/health');
        $this->assertSame(503, $response->getStatusCode());
        $this->assertSame('', $response->getContent());
        $this->assertError($this->send('PUT', '/api/workspace', ['Origin' => 'https://evil.example', 'Content-Type' => 'application/json'], '{}'), 403, 'forbiddenRequest');
        $this->assertError($this->send('POST', '/api/workspace/records', ['Origin' => self::ORIGIN, 'Content-Type' => 'Application/JSON'], '{}'), 403, 'forbiddenRequest');
        $this->assertError($this->send('GET', '/api/workspace'), 503, 'databaseUnavailable');
        config(['typle.auth_url' => null]);
        $this->assertError($this->send('GET', '/api/workspace'), 503, 'authUnavailable');
    }

    public function test_an_apps_bearer_token_is_not_a_login(): void
    {
        config(['typle.database_url' => null]);
        $this->assertError($this->send('PUT', '/api/workspace', ['Authorization' => 'Bearer x', 'Content-Type' => 'application/json'], '{}'), 403, 'forbiddenRequest');
    }

    // With a database

    public function test_workspace_writes_as_the_go_backend(): void
    {
        $this->database();
        foreach (Fixtures::read('workspace-writes.json')->scenarios as $scenario) {
            $owner = $this->newOwner();
            $row = Value::prop(Value::prop($scenario, 'setup'), 'row');
            if ($row instanceof stdClass) {
                $this->insertRow($owner, $row);
            }
            foreach ($scenario->steps as $index => $step) {
                $message = "{$scenario->name}: step ".($index + 1);
                $this->assertSame($step->result, Saves::save($owner, $step->body), $message);
                $this->assertSame(self::comparable(['row' => $step->row, 'records' => $step->records]), self::comparable($this->state($owner)), $message);
            }
        }
    }

    public function test_record_writes_as_the_go_backend(): void
    {
        $this->database();
        foreach (Fixtures::read('record-writes.json')->scenarios as $scenario) {
            $owner = $this->newOwner();
            $row = Value::prop(Value::prop($scenario, 'setup'), 'row');
            if ($row instanceof stdClass) {
                $this->insertRow($owner, $row);
            }
            foreach ($scenario->steps as $index => $step) {
                $message = "{$scenario->name}: step ".($index + 1);
                $body = Value::object(['accountId' => $owner]);
                foreach ($step->body as $key => $item) {
                    $body->{$key} = $item;
                }
                $response = $this->send($step->kind === 'add' ? 'POST' : 'DELETE', '/api/workspace/records', self::headersFor($owner), JsonValue::generate($body));
                $this->assertSame($step->status, $response->getStatusCode(), "{$message}: {$response->getContent()}");
                if ($step->key !== '') {
                    $this->assertSame(self::comparable(['error' => $step->key]), self::comparable(self::answer($response)), $message);
                }
                if ($step->kind === 'add' && $step->key === '') {
                    $this->assertSame(self::comparable($step->record), self::comparable(self::answer($response)->record), $message);
                }
                $this->assertSame(self::comparable(['row' => $step->row, 'records' => $step->records]), self::comparable($this->state($owner)), $message);
            }
        }
    }

    public function test_an_account_that_has_saved_nothing_reads_as_revision_0_and_a_save_is_read_back(): void
    {
        $this->database();
        $owner = $this->newOwner();
        $response = $this->send('GET', '/api/workspace', self::headersFor($owner));
        $this->assertSame(200, $response->getStatusCode());
        $this->assertSame(self::comparable(['lists' => [], 'packs' => null, 'devices' => [], 'interests' => Interests::defaults(), 'revision' => 0, 'accountId' => $owner]),
            self::comparable(self::answer($response)));
        $response = $this->send('PUT', '/api/workspace', self::headersFor($owner), '{"revision":0,"accountId":"'.$owner.'",'.self::library('one').'}');
        $this->assertSame(200, $response->getStatusCode(), $response->getContent());
        $this->assertSame(1, self::answer($response)->revision);
        $saved = $response->getContent();
        $this->assertSame($saved, $this->send('GET', '/api/workspace', self::headersFor($owner))->getContent());
        $this->assertError($this->send('PUT', '/api/workspace', self::headersFor($owner), '{"revision":0,"accountId":"'.$owner.'",'.self::library('two').'}'), 409, 'conflict');
        // PUT always writes: the same workspace sent back at its revision moves the revision on.
        $response = $this->send('PUT', '/api/workspace', self::headersFor($owner), '{"revision":1,"accountId":"'.$owner.'",'.self::library('one').'}');
        $this->assertSame(2, self::answer($response)->revision);
    }

    public function test_a_save_is_refused_as_the_go_backend_refused_it(): void
    {
        $this->database();
        $owner = $this->newOwner();
        $good = '{"revision":0,"accountId":"'.$owner.'",'.self::library('one').'}';
        $this->assertError($this->send('PUT', '/api/workspace', self::headersFor($owner, ['Origin' => 'https://evil.example']), $good), 403, 'forbiddenRequest');
        $this->assertError($this->send('PUT', '/api/workspace', self::headersFor($owner, ['Content-Type' => 'text/plain']), $good), 403, 'forbiddenRequest');
        $this->assertError($this->send('PUT', '/api/workspace', self::headersFor(null), $good), 401, 'loginRequired');
        $this->assertError($this->send('PUT', '/api/workspace', self::headersFor($owner), ''), 400, 'emptyRequest');
        $this->assertError($this->send('PUT', '/api/workspace', self::headersFor($owner), '{"x":"'.str_repeat('a', 1 << 20).'"}'), 413, 'workspaceTooLarge');
        $this->assertError($this->send('PUT', '/api/workspace', self::headersFor($owner), '[]'), 400, 'invalidWorkspace');
        $this->assertError($this->send('PUT', '/api/workspace', self::headersFor($owner), '{'), 400, 'invalidWorkspace');
        $this->assertError($this->send('PUT', '/api/workspace', self::headersFor($owner), '{"revision":0,"accountId":"'.$owner.'","lists":[{"id":"a","name":"a\ud800"}]}'), 400, 'invalidWorkspace');
        $this->assertError($this->send('PUT', '/api/workspace', self::headersFor($owner), '{"revision":0,"accountId":"github:2",'.self::library('one').'}'), 401, 'loginAccountChanged');
    }

    public function test_a_session_is_read_from_its_row_while_it_has_not_expired_and_signing_out_of_every_device_ends_it(): void
    {
        $this->database();
        $owner = $this->newOwner();
        $response = $this->send('GET', '/api/workspace', self::headersFor($owner));
        $this->assertSame(200, $response->getStatusCode(), $response->getContent());
        $this->assertEquals(new Session($owner, 'Test', ''), AuthSession::userOf(self::tokenOf($owner), Date::now()));
        $this->assertNull(AuthSession::userOf(self::tokenOf($owner).'x', Date::now()));
        $this->assertNull(AuthSession::userOf(self::tokenOf($owner), Date::now()->addDays(2)));
        // Expiry is inclusive: a session is read until its expires_at.
        $expiresAt = AuthSession::query()->where('token', self::tokenOf($owner))->toBase()->value('expires_at');
        $this->assertSame($owner, AuthSession::userOf(self::tokenOf($owner), $expiresAt)?->sub);
        // Signing out of every device deletes the rows: the cookie no longer opens, and it is not removed.
        AuthSession::query()->where('user_id', $owner)->delete();
        $response = $this->send('GET', '/api/workspace', self::headersFor($owner));
        $this->assertError($response, 401, 'loginRequired');
        $this->assertNull($response->headers->get('Set-Cookie'));
    }

    public function test_an_expired_session_is_refused(): void
    {
        $this->database();
        $owner = $this->newOwner();
        AuthSession::query()->where('user_id', $owner)->update(['expires_at' => Date::now()->subMinute()->format('Y-m-d\TH:i:s.uP')]);
        $this->assertError($this->send('GET', '/api/workspace', self::headersFor($owner)), 401, 'loginRequired');
    }

    /** PHP has no threads: the two saves run in two processes, each with its own connection. */
    public function test_of_two_saves_from_the_same_revision_only_one_is_saved(): void
    {
        $this->database();
        $owner = $this->newOwner();
        $body = static fn (int $revision, string $name) => '{"revision":'.$revision.','.self::library($name).'}';
        $run = function (int $revision) use ($owner, $body): array {
            $env = ['APP_ENV' => 'testing', 'TYPLE_TEST_DATABASE_URL' => getenv('TYPLE_TEST_DATABASE_URL')];
            $processes = array_map(static fn (string $name) => new Process([PHP_BINARY, 'tests/Support/save.php', $owner, $body($revision, $name)], base_path(), $env),
                ['one', 'two']);
            array_walk($processes, static fn (Process $process) => $process->start());
            $results = array_map(static function (Process $process): string {
                $process->wait();

                return $process->isSuccessful() ? $process->getOutput() : 'failed: '.$process->getErrorOutput().$process->getOutput();
            }, $processes);
            sort($results);

            return $results;
        };
        $this->assertSame(['conflict', 'ok'], $run(0));
        $this->assertSame(['conflict', 'ok'], $run(1));
        $this->assertSame(2, $this->state($owner)->row->revision);
    }

    public function test_the_health_answers_200_once_the_database_answers(): void
    {
        $this->database();
        $response = $this->send('GET', '/api/health');
        $this->assertSame(200, $response->getStatusCode());
        $this->assertSame("{\"status\":\"ok\"}\n", $response->getContent());
        $this->assertSame('private, no-store', $response->headers->get('Cache-Control'));
    }

    // Review focus

    public function test_integral_floats_are_saved_and_answered_as_js_writes_them(): void
    {
        $this->database();
        $owner = $this->newOwner();
        $record = '{"id":"r","time":1500.0,"date":"'.self::AT.'","mistakes":0,"accuracy":100.0,"wordCount":1}';
        $response = $this->send('PUT', '/api/workspace', self::headersFor($owner),
            '{"revision":0,"accountId":"'.$owner.'",'.str_replace('"records":[]', '"records":['.$record.']', self::library('one')).'}');
        $this->assertSame(200, $response->getStatusCode(), $response->getContent());
        $this->assertStringContainsString('"time":1500,', $response->getContent());
        $this->assertSame('{"id": "r", "date": "2026-09-07T00:00:00.000Z", "time": 1500, "accuracy": 100, "mistakes": 0, "wordCount": 1}',
            DB::selectOne('SELECT data::text AS data FROM typle_records WHERE owner_id = ?', [$owner])->data);
    }

    public function test_a_body_of_exactly_1_mib_is_read_one_byte_more_is_413(): void
    {
        $this->database();
        $owner = $this->newOwner();
        $this->assertError($this->send('PUT', '/api/workspace', self::headersFor($owner), '{"x":"'.str_repeat('a', (1 << 20) - 8).'"}'), 401, 'loginAccountChanged');
        $this->assertError($this->send('PUT', '/api/workspace', self::headersFor($owner), '{"x":"'.str_repeat('a', (1 << 20) - 7).'"}'), 413, 'workspaceTooLarge');
    }

    public function test_a_body_is_read_as_text_decoder_reads_it_the_bom_dropped_malformed_bytes_as_u_fffd(): void
    {
        $this->database();
        $owner = $this->newOwner();
        $response = $this->send('PUT', '/api/workspace', self::headersFor($owner), "\xEF\xBB\xBF{\"revision\":0,\"accountId\":\"{$owner}\",".self::library('one').'}');
        $this->assertSame(200, $response->getStatusCode(), $response->getContent());
        $response = $this->send('PUT', '/api/workspace', self::headersFor($owner), "{\"revision\":1,\"accountId\":\"{$owner}\",".self::library("o\xFFe").'}');
        $this->assertSame(200, $response->getStatusCode(), $response->getContent());
        $this->assertSame("o\u{FFFD}e", self::answer($response)->lists[0]->name);
    }

    public function test_an_unexpected_error_is_a_json_503_with_the_headers(): void
    {
        $this->database();
        $owner = $this->newOwner();
        $this->app->instance(WorkspaceStore::class, new class extends WorkspaceStore
        {
            public function write(string $owner, int $revision, WorkspaceChange $change): ?Saved
            {
                throw new RuntimeException('boom');
            }
        });
        $response = $this->send('PUT', '/api/workspace', self::headersFor($owner), '{"revision":0,"accountId":"'.$owner.'",'.self::library('one').'}');
        $this->assertError($response, 503, 'workspaceSaveFailed');
        $this->assertSame('laravel', $response->headers->get('x-typle-backend'));
        $this->app->instance(WorkspaceStore::class, new class extends WorkspaceStore
        {
            public function read(string $owner): ?Saved
            {
                throw new RuntimeException('boom');
            }
        });
        $this->assertError($this->send('GET', '/api/workspace', self::headersFor($owner)), 503, 'databaseUnavailable');
    }

    public function test_each_request_sends_the_statements_the_next_js_handlers_send(): void
    {
        $this->database();
        $owner = $this->newOwner();
        $this->send('PUT', '/api/workspace', self::headersFor($owner), '{"revision":0,"accountId":"'.$owner.'",'.self::library('one').'}');
        $counted = function (callable $request): array {
            $statements = [];
            $listening = true;
            DB::listen(function (QueryExecuted $query) use (&$statements, &$listening) {
                if ($listening) {
                    $statements[] = $query->sql;
                }
            });
            $request();
            $listening = false;

            return $statements;
        };
        // The session with its user, then the workspace with its records.
        $this->assertCount(2, $counted(fn () => $this->send('GET', '/api/workspace', self::headersFor($owner))));
        // The session, the revision and counts, the update, the records' delete and count (no rows to upsert), the read back.
        $put = $counted(fn () => $this->send('PUT', '/api/workspace', self::headersFor($owner), '{"revision":1,"accountId":"'.$owner.'",'.self::library('one').'}'));
        $this->assertCount(6, $put, implode("\n", $put));
        // The session, the row held for update, its records, the update, the records' delete, upsert and count, the read back.
        $add = '{"accountId":"'.$owner.'","listId":"l","record":{"id":"r1","time":1,"date":"'.self::AT.'","mistakes":0,"accuracy":100}}';
        $response = null;
        $statements = $counted(function () use (&$response, $add, $owner) {
            $response = $this->send('POST', '/api/workspace/records', self::headersFor($owner), $add);
        });
        $this->assertSame(201, $response->getStatusCode(), $response->getContent());
        $this->assertCount(8, $statements, implode("\n", $statements));
    }
}
