<?php

declare(strict_types=1);

namespace App\Models;

use Illuminate\Database\Eloquent\Model;

/**
 * A user of Better Auth (typle_auth_users, migration 0042 of typle-r): its id is the account ID, the owner_id of the
 * account's data. The table belongs to typle-r.
 */
class AuthUser extends Model
{
    protected $table = 'typle_auth_users';

    protected $keyType = 'string';

    public $incrementing = false;

    public $timestamps = false;

    protected $guarded = [];
}
