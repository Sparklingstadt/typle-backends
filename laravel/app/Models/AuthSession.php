<?php

declare(strict_types=1);

namespace App\Models;

use App\Typle\Session;
use DateTimeInterface;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

/**
 * A session of Better Auth (typle_auth_sessions, migration 0042 of typle-r): the login cookie names one by its token, and
 * signing out of every device deletes the user's rows. The table belongs to typle-r.
 */
class AuthSession extends Model
{
    protected $table = 'typle_auth_sessions';

    protected $keyType = 'string';

    public $incrementing = false;

    public $timestamps = false;

    protected $guarded = [];

    public function user(): BelongsTo
    {
        return $this->belongsTo(AuthUser::class, 'user_id');
    }

    /**
     * The user of the session whose token is token, as Better Auth's findSession and getSession read it: the session's
     * row and its user's, in one statement, and only while the session has not expired at now (expires_at not before
     * now). null for a token of no session and for an expired one. now is a time, or a timestamptz as PostgreSQL wrote it.
     */
    public static function userOf(string $token, DateTimeInterface|string $now): ?Session
    {
        $users = (new AuthUser)->getTable();
        $row = static::query()
            ->join($users, "{$users}.id", '=', 'typle_auth_sessions.user_id')
            ->where('token', $token)
            ->where('expires_at', '>=', $now instanceof DateTimeInterface ? $now->format('Y-m-d\TH:i:s.uP') : $now)
            ->toBase()
            ->first(["{$users}.id", "{$users}.name", "{$users}.image"]);

        return $row === null ? null : new Session($row->id, $row->name, (string) $row->image);
    }
}
