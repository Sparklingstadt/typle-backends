<?php

namespace Tests\Lib;

use App\Typle\LoginSettings;
use App\Typle\Sessions;
use Tests\TestCase;

/**
 * Better Auth's session cookie as better-call signs it (server/internal/session/session_test.go checks the same values),
 * and the login settings of the environment.
 */
class SessionsTest extends TestCase
{
    /** Values better-call's signCookieValue (better-call 1.x in typle-r v2.68.1) signed with this secret. */
    private const SECRET = 'typle-fixture-secret';

    private const SIGNED = [
        ['Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj', 'Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D'],
        ['a.b+c/d=e', 'a.b%2Bc%2Fd%3De.QgtT95hjCvQfprQC%2BUtI5LsqCC549MK9KWYHFNPmUuE%3D'],
    ];

    private function settings(string $url = 'https://typle.example', ?string $secret = 's', ?string $demo = null): ?LoginSettings
    {
        return LoginSettings::read(['AUTH_URL' => $url, 'BETTER_AUTH_SECRET' => $secret, 'DEMO_LOGIN' => $demo]);
    }

    public function test_a_token_is_signed_as_better_call_signs_it(): void
    {
        foreach (self::SIGNED as [$token, $value]) {
            $this->assertSame($value, Sessions::sign($token, self::SECRET), $token);
        }
    }

    public function test_a_signed_value_opens_to_its_token(): void
    {
        foreach (self::SIGNED as [$token, $value]) {
            $this->assertSame($token, Sessions::token($value, self::SECRET), $value);
        }
        // The value as it is, not URL-encoded, opens too: decoding leaves it as it is.
        $this->assertSame(self::SIGNED[0][0], Sessions::token('Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK+RyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w=', self::SECRET));
        // A value that is not valid percent-encoding is read as it is (tryDecode).
        $this->assertNull(Sessions::token('%zz.'.self::SIGNED[0][1], self::SECRET));
    }

    public function test_any_other_value_is_refused(): void
    {
        $good = self::SIGNED[0][1];
        foreach ([
            'empty' => '',
            'no dot' => 'Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj',
            'dot first' => '.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D',
            'other token' => 'Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHk.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D',
            'short' => 'Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w',
            'no padding end' => 'Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK%2BRyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2wA',
            'not base64' => 'Zq3kV8wXo1pN6tYb2cR9sL4mA7dF0gHj.HaFpK-RyVHSMXc5MbMT65Lkh4SdoCpRqCHG866eQx2w%3D',
        ] as $name => $value) {
            $this->assertNull(Sessions::token($value, self::SECRET), $name);
        }
        $this->assertNull(Sessions::token($good, 'another secret'));
        $this->assertNull(Sessions::token($good, ''));
    }

    public function test_the_cookies_name_follows_auth_url(): void
    {
        $this->assertSame('better-auth.session_token', Sessions::cookieName(false));
        $this->assertSame('__Secure-better-auth.session_token', Sessions::cookieName(true));
        $this->assertSame('__Secure-better-auth.session_token', $this->settings()->cookieName());
        $this->assertSame('better-auth.session_token', $this->settings('http://127.0.0.1:3101')->cookieName());
    }

    public function test_the_settings_need_an_http_url_the_secret_is_trimmed_and_demo_login_must_be_exactly_1(): void
    {
        $this->assertSame('http://127.0.0.1:3101', $this->settings('http://127.0.0.1:3101/x')->origin);
        $this->assertSame('https://typle.example', $this->settings('HTTPS://Typle.example:443/')->origin);
        $this->assertNull($this->settings('ftp://typle.example'));
        $this->assertNull($this->settings(''));
        $this->assertSame('', $this->settings(secret: null)->secret);
        $this->assertSame('', $this->settings(secret: " \n")->secret);
        $this->assertSame('a b', $this->settings(secret: " a b\t")->secret);
        $this->assertTrue($this->settings(demo: '1')->demoLogin);
        $this->assertFalse($this->settings(demo: 'true')->demoLogin);
        $this->assertFalse($this->settings()->demoLogin);
    }

    public function test_the_cookie_is_read_from_the_cookie_header_as_better_calls_parse_cookies_reads_it(): void
    {
        $name = 'better-auth.session_token';
        $this->assertSame('v', Sessions::cookieValue("a=1; {$name}=v; {$name}=w", $name));
        $this->assertSame('v', Sessions::cookieValue(" {$name} = \"v\" ", $name));
        // A leading quote drops the first and the last character, whatever the last one is.
        $this->assertSame('a', Sessions::cookieValue("{$name}=\"ab", $name));
        $this->assertSame('', Sessions::cookieValue("{$name}=\"", $name));
        // The value is not decoded here.
        $this->assertSame('a%2Bb', Sessions::cookieValue("{$name}=a%2Bb", $name));
        $this->assertNull(Sessions::cookieValue("__Secure-{$name}=v", $name));
        $this->assertNull(Sessions::cookieValue('other=1', $name));
        $this->assertNull(Sessions::cookieValue(null, $name));
    }
}
