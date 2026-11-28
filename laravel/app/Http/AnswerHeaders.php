<?php

declare(strict_types=1);

namespace App\Http;

use Symfony\Component\HttpFoundation\ResponseHeaderBag;

/**
 * Response headers that keep Cache-Control as it was set.
 */
final class AnswerHeaders extends ResponseHeaderBag
{
    protected function computeCacheControlValue(): string
    {
        $given = implode(', ', $this->headers['cache-control'] ?? []);

        return $given === '' ? parent::computeCacheControlValue() : $given;
    }
}
