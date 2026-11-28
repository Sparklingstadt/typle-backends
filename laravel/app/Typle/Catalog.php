<?php

declare(strict_types=1);

namespace App\Typle;

use RuntimeException;

/**
 * The official packs (src/lib/packs/catalog.ts and index.ts), built once per process from the JSON of src/lib/packs.
 * The files are read where they are in the repository, or under TYPLE_APP_DATA (the Docker image copies them there).
 * Octane keeps the process, so the catalog is built once per worker.
 */
final class Catalog
{
    /** @var list<OfficialPack>|null */
    private static ?array $official = null;

    private static ?string $defaultDeviceName = null;

    public static function root(): string
    {
        $data = config('typle.app_data');

        return $data !== null && $data !== '' ? $data : dirname(base_path());
    }

    public static function read(string $path): mixed
    {
        return JsonValue::parse(file_get_contents(self::root().'/'.$path));
    }

    /** @return list<OfficialPack> */
    public static function official(): array
    {
        return self::$official ??= self::build();
    }

    /** followed of catalog.ts: the library in line with the catalog. */
    public static function followed(Library $library): Library
    {
        return FollowedPacks::followOfficial($library, self::official());
    }

    /** The built-in device's name in the app's base language (src/messages/ja/format.json defaultDevice). */
    public static function defaultDeviceName(): string
    {
        return self::$defaultDeviceName ??= self::read('src/messages/ja/format.json')->defaultDevice;
    }

    /** @return list<OfficialPack> */
    private static function build(): array
    {
        $library = WordPacks::convertLibrary(self::officialLists());
        $official = [];
        foreach ($library->packs as $pack) {
            $pack = Value::merge($pack, ['origin' => Value::object(['catalogId' => $pack->id, 'version' => 1])]);
            $lists = array_values(array_filter($library->lists, static fn ($list) => $list->packId === $pack->id));
            $official[] = new OfficialPack($pack->id, 1, $pack, $lists);
        }

        return $official;
    }

    /** initialLists of src/lib/packs/index.ts: samples (numbered), countries, then the commands of manifest.json. */
    private static function officialLists(): array
    {
        $samples = [];
        foreach (self::read('src/lib/packs/samples.json') as $index => $list) {
            $samples[] = Value::merge($list, ['id' => 'sample-'.($index + 1), 'records' => []]);
        }
        $commands = [];
        foreach (self::read('src/lib/packs/commands/manifest.json') as $name) {
            $list = self::read("src/lib/packs/commands/{$name}");
            foreach ($list->words as $word) {
                if (! WordLists::inputType(Value::prop($word, 'input_type'))) {
                    throw new RuntimeException('Invalid official input_type');
                }
            }
            $commands[] = $list;
        }

        return [...$samples, ...self::read('src/lib/packs/countries.json'), ...$commands];
    }
}
