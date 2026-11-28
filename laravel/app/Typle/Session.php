<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * A signed-in user: sub is the account ID (typle_auth_users.id, the owner_id of the account's data).
 */
final readonly class Session
{
    public function __construct(public string $sub, public string $name, public string $image) {}
}
