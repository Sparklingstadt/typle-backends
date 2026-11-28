<?php

namespace Tests;

use App\Typle\JsonValue;

/**
 * The shared fixtures of the repository (tests/fixtures), read in place: Next.js, Go, Kotlin and Rails are held to the
 * same files.
 */
final class Fixtures
{
    public static function dir(): string
    {
        return dirname(__DIR__, 2).'/tests/fixtures';
    }

    public static function read(string $path): mixed
    {
        return JsonValue::parse(file_get_contents(self::dir().'/'.$path));
    }

    /**
     * The JSON files of a fixture directory, by name.
     *
     * @return array<string, array{mixed}>
     */
    public static function files(string $path): array
    {
        $files = glob(self::dir().'/'.$path.'/*.json');
        sort($files);
        $cases = [];
        foreach ($files as $file) {
            $cases[basename($file)] = [JsonValue::parse(file_get_contents($file))];
        }

        return $cases;
    }
}
