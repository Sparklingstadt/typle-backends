<?php

declare(strict_types=1);

namespace App\Http;

use App\Typle\JsonValue;
use Symfony\Component\HttpFoundation\Response;

/**
 * A JSON answer as the Next.js handlers write one (src/server/response.ts): JSON.stringify plus a newline, never cached,
 * varying by Cookie. Its headers are written as they are given (Symfony would reorder Cache-Control's directives into
 * "no-store, private").
 */
final class Answer extends Response
{
    public function __construct(?string $content = '', int $status = 200, array $headers = [])
    {
        parent::__construct($content, $status);
        $this->headers = new AnswerHeaders($headers);
    }

    public static function json(int $status, mixed $value): self
    {
        return new self(JsonValue::generate($value)."\n", $status, [
            'content-type' => 'application/json; charset=utf-8',
            'cache-control' => 'private, no-store',
            'vary' => 'Cookie',
        ]);
    }

    public static function error(int $status, string $key): self
    {
        return self::json($status, ['error' => $key]);
    }
}
