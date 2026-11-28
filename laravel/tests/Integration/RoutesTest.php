<?php

namespace Tests\Integration;

use App\Typle\Sessions;
use Tests\TestCase;

/**
 * The routes without a database: the gate, the headers every answer carries and the methods each path serves.
 */
class RoutesTest extends TestCase
{
    public function test_outside_the_comparison_every_request_is_404_not_found(): void
    {
        config(['typle.compare' => false]);
        foreach ([['GET', '/api/health'], ['GET', '/api/workspace'], ['PUT', '/api/workspace'], ['POST', '/api/workspace/records'], ['GET', '/']] as [$method, $path]) {
            $response = $this->send($method, $path);
            $this->assertSame(404, $response->getStatusCode(), "{$method} {$path}");
            $this->assertSame("{\"error\":\"notFound\"}\n", $response->getContent());
            $this->assertSame('laravel', $response->headers->get('x-typle-backend'));
            $this->assertSame('DENY', $response->headers->get('X-Frame-Options'));
            $this->assertSame("frame-ancestors 'none'; base-uri 'self'; object-src 'none'", $response->headers->get('Content-Security-Policy'));
        }
    }

    public function test_other_methods_are_405_with_allow(): void
    {
        config(['typle.compare' => true]);
        foreach ([
            '/api/health' => [['POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS'], 'GET, HEAD'],
            '/api/workspace' => [['POST', 'PATCH', 'DELETE', 'OPTIONS'], 'GET, HEAD, PUT'],
            '/api/workspace/records' => [['GET', 'HEAD', 'PUT', 'PATCH', 'OPTIONS'], 'POST, DELETE'],
        ] as $path => [$methods, $allow]) {
            foreach ($methods as $method) {
                $response = $this->send($method, $path);
                $this->assertSame(405, $response->getStatusCode(), "{$method} {$path}");
                $this->assertSame($allow, $response->headers->get('Allow'));
                $this->assertSame('private, no-store', $response->headers->get('Cache-Control'));
                $this->assertSame('laravel', $response->headers->get('x-typle-backend'));
                $this->assertSame($method === 'HEAD' ? '' : "{\"error\":\"methodNotAllowed\"}\n", $response->getContent());
            }
        }
    }

    public function test_an_unknown_path_in_the_comparison_is_404_not_found(): void
    {
        config(['typle.compare' => true]);
        $response = $this->send('GET', '/api/other');
        $this->assertSame(404, $response->getStatusCode());
        $this->assertSame("{\"error\":\"notFound\"}\n", $response->getContent());
        $this->assertSame('laravel', $response->headers->get('x-typle-backend'));
    }

    public function test_the_answers_carry_the_headers_of_next_js_and_no_others_of_laravel(): void
    {
        config(['typle.compare' => true]);
        foreach ([['POST', '/api/health'], ['GET', '/api/other']] as [$method, $path]) {
            $response = $this->send($method, $path, ['Accept' => 'text/html']);
            $this->assertSame('Cookie', $response->headers->get('Vary'), "{$method} {$path}");
            $this->assertNull($response->headers->get('X-XSS-Protection'));
            $this->assertNull($response->headers->get('X-Permitted-Cross-Domain-Policies'));
            $this->assertSame('application/json; charset=utf-8', $response->headers->get('Content-Type'));
            $this->assertSame([], $response->headers->getCookies());
        }
    }

    public function test_a_header_with_bytes_beyond_ascii_is_read_as_node_reads_it_not_refused_with_a_503(): void
    {
        config(['typle.compare' => true, 'typle.better_auth_secret' => 's', 'typle.auth_url' => 'http://127.0.0.1:3101']);
        $response = $this->send('PUT', '/api/workspace', ['Origin' => 'http://127.0.0.1:3101', 'Content-Type' => "applicatio\xE9/json"], '{}');
        $this->assertError($response, 403, 'forbiddenRequest');
        $this->assertSame('v', Sessions::cookieValue("\xC3\xA9=1; better-auth.session_token=v", 'better-auth.session_token'));
        $this->assertSame("ab\u{00C3}\u{00A9}", Sessions::cookieValue("better-auth.session_token=ab\xC3\xA9", 'better-auth.session_token'));
    }

    // Beyond the Rails tests: what Laravel does otherwise unless it is told not to.

    public function test_a_method_is_never_taken_from_an_override(): void
    {
        config(['typle.compare' => true]);
        $response = $this->send('POST', '/api/health?_method=GET', ['X-HTTP-Method-Override' => 'GET']);
        $this->assertError($response, 405, 'methodNotAllowed');
        $response = $this->send('POST', '/api/health', ['Content-Type' => 'application/json'], '{"_method":"GET"}');
        $this->assertError($response, 405, 'methodNotAllowed');
        $response = $this->send('POST', '/api/health', ['Content-Type' => 'application/x-www-form-urlencoded'], '_method=GET');
        $this->assertError($response, 405, 'methodNotAllowed');
    }

    public function test_a_method_laravel_has_no_route_verb_for_is_405_on_a_known_path_and_404_elsewhere(): void
    {
        config(['typle.compare' => true]);
        $response = $this->send('PROPFIND', '/api/workspace');
        $this->assertError($response, 405, 'methodNotAllowed');
        $this->assertSame('GET, HEAD, PUT', $response->headers->get('Allow'));
        $this->assertError($this->send('PROPFIND', '/api/other'), 404, 'notFound');
        $this->assertError($this->send('POST', '/api/other'), 404, 'notFound');
        $this->assertError($this->send('GET', '/storage/x'), 404, 'notFound');
        $this->assertSame('', $this->send('HEAD', '/api/other')->getContent());
    }
}
