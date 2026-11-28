<?php

declare(strict_types=1);

namespace App\Typle;

/**
 * src/lib/limits.ts (checked against tests/fixtures/limits.json).
 */
final class Limits
{
    public const MAX_LISTS = 150;
    public const MAX_FOLLOWED_PACKS = 200;
    public const MAX_LIST_NAME = 200;
    public const MAX_WORDS = 1000;
    public const MAX_RECORDS = 10000;
    public const MAX_CATEGORY = 50;
    public const MAX_TAG = 50;
    public const MAX_TAGS = 20;
    public const MAX_GROUP = 200;
    public const MAX_LEVELS = 1;
    public const MAX_SAVED_LEVELS = 5;
    public const MAX_LEVEL = 50;
    public const MAX_INTERESTS = 50;
    public const MAX_INTEREST_LENGTH = 50;
    public const MAX_PACK_NAME = 200;
    public const MAX_PACK_DESCRIPTION = 500;
    public const MAX_DEVICES = 100;
    public const MAX_DEVICE_NAME = 100;
    public const MAX_DEVICE_LAYOUT = 200;
    public const MAX_MISSED_WORDS = 20;
    public const MAX_WEAK_REVIEW_MISSED = 200;
    public const MAX_WORD_TEXT = 1000;
    public const MAX_ANNOTATION = 2000;
    public const MAX_RECORD_ID = 100;
    public const MAX_NAME = 200;
}
