module Typle
  # The official packs (src/lib/packs/catalog.ts and index.ts), built once from the JSON of src/lib/packs. The files are
  # read where they are in the repository, or under TYPLE_APP_DATA (the Docker image copies them there).
  module Catalog
    LOCK = Mutex.new

    module_function

    def root = Pathname(ENV["TYPLE_APP_DATA"].presence || Rails.root.join(".."))

    def read(path) = JsonValue.parse(File.read(root.join(path)))

    def official
      @official || LOCK.synchronize { @official ||= build }
    end

    # followed of catalog.ts: the library in line with the catalog.
    def followed(library) = FollowedPacks.follow_official(library, official)

    # The built-in device's name in the app's base language (src/messages/ja/format.json defaultDevice).
    def default_device_name
      @default_device_name ||= read("src/messages/ja/format.json")["defaultDevice"]
    end

    def build
      library = WordPacks.convert_library(official_lists)
      library.packs.map do |pack|
        pack = pack.merge("origin" => { "catalogId" => pack["id"], "version" => 1 })
        OfficialPack.new(pack["id"], 1, pack, library.lists.select { |list| list["packId"] == pack["id"] })
      end.freeze
    end

    # initialLists of src/lib/packs/index.ts: samples (numbered), countries, then the commands of manifest.json.
    def official_lists
      samples = read("src/lib/packs/samples.json").each_with_index.map do |list, index|
        list.merge("id" => "sample-#{index + 1}", "records" => [])
      end
      commands = read("src/lib/packs/commands/manifest.json").map do |name|
        list = read("src/lib/packs/commands/#{name}")
        list["words"].each { |word| raise "Invalid official input_type" unless WordLists.input_type?(word["input_type"]) }
        list
      end
      samples + read("src/lib/packs/countries.json") + commands
    end
  end
end
