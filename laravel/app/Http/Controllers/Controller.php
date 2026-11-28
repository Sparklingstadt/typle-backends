<?php

declare(strict_types=1);

namespace App\Http\Controllers;

use App\Http\Answer;
use App\Http\Middleware\RequireOwner;
use App\Typle\JsonValue;
use Illuminate\Http\Request;
use JsonException;
use stdClass;

/**
 * What the Next.js handlers share (src/server/response.ts, request.ts and the body of a save in workspace.ts): the JSON
 * answers and the reading of a save's body. The checks before a handler (login settings, a write's Origin and
 * Content-Type, the database, the login) are the middleware of routes/api.php.
 */
abstract class Controller
{
    public const MAX_BODY_BYTES = 1 << 20;

    private const BOM = "\xEF\xBB\xBF";

    protected function reply(int $status, mixed $value): Answer
    {
        return Answer::json($status, $value);
    }

    protected function refuse(int $status, string $key): Answer
    {
        return Answer::error($status, $key);
    }

    /** The account ID of the login (RequireOwner). */
    protected function owner(Request $request): string
    {
        return $request->attributes->get(RequireOwner::ATTRIBUTE);
    }

    /** readSaveRequest: the body as an object whose accountId is the owner's, or the refusal to answer. */
    protected function saveFields(Request $request, string $invalid): stdClass|Answer
    {
        $body = $request->getContent();
        if ($body === '') {
            return $this->refuse(400, 'emptyRequest');
        }
        if (strlen($body) > self::MAX_BODY_BYTES) {
            return $this->refuse(413, 'workspaceTooLarge');
        }

        try {
            $fields = JsonValue::parse(self::decodeBody($body));
        } catch (JsonException) {
            $fields = null;
        }
        if (! $fields instanceof stdClass) {
            return $this->refuse(400, $invalid);
        }
        if (! property_exists($fields, 'accountId') || $fields->accountId !== $this->owner($request)) {
            return $this->refuse(401, 'loginAccountChanged');
        }

        return $fields;
    }

    /** The body as new TextDecoder().decode reads it: UTF-8, a leading BOM dropped, malformed bytes as U+FFFD. */
    private static function decodeBody(string $bytes): string
    {
        if (str_starts_with($bytes, self::BOM)) {
            $bytes = substr($bytes, 3);
        }
        if (mb_check_encoding($bytes, 'UTF-8')) {
            return $bytes;
        }
        $substitute = mb_substitute_character();
        mb_substitute_character(0xFFFD);
        try {
            return mb_scrub($bytes, 'UTF-8');
        } finally {
            mb_substitute_character($substitute);
        }
    }
}
