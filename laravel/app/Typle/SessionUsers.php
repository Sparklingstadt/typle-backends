<?php

declare(strict_types=1);

namespace App\Typle;

use App\Models\AuthSession;
use DateTimeInterface;

/**
 * The users of Better Auth's sessions (AuthSession::userOf), as a service of the container so the tests can stand in for
 * the sessions without a database.
 */
class SessionUsers
{
    public function userOf(string $token, DateTimeInterface|string $now): ?Session
    {
        return AuthSession::userOf($token, $now);
    }
}
