module Typle
  # src/lib/followedPacks.ts: keeping followed official packs in line with the catalog.
  module FollowedPacks
    module_function

    def followed?(pack) = Value.prop(pack["origin"], "followed") == true

    # The lists as they are saved: those of a followed pack without their words (withoutFollowedWords).
    def without_followed_words(library)
      followed = library.packs.select { |pack| followed?(pack) }.to_set { |pack| pack["id"] }
      Library.new(library.packs, library.lists.map do |list|
        followed.include?(list["packId"]) && Value.truthy?(list["origin"]) ? list.merge("words" => []) : list
      end)
    end

    # followOfficial: removes copies from before v0.98.0 and makes each followed pack equal to its official pack.
    def follow_official(library, official)
      catalog = official.to_h { |entry| [entry.catalog_id, entry] }
      kept = library.packs.select do |pack|
        origin = pack["origin"]
        if Value.truthy?(origin)
          followed?(pack) && catalog.key?(Value.prop(origin, "catalogId"))
        else
          !catalog.key?(pack["id"])
        end
      end
      packs = []
      synced = {}
      kept.each do |pack|
        unless followed?(pack)
          packs << pack
          next
        end
        entry = catalog[pack["origin"]["catalogId"]]
        rest = pack.except("description", "category", "tags")
        %w[description category].each { |key| rest[key] = entry.pack[key] if entry.pack[key].is_a?(String) && !entry.pack[key].empty? }
        rest["tags"] = entry.pack["tags"].dup if entry.pack["tags"].is_a?(Array) && !entry.pack["tags"].empty?
        packs << rest
        pack_id = pack["id"]
        mine = library.lists.select { |list| list["packId"] == pack_id }
        synced[pack_id] = entry.lists.map do |item|
          found = mine.find { |list| Value.prop(list["origin"], "listId") == item["id"] }
          base = found || {
            "id" => "#{pack_id}:#{item["id"]}", "packId" => pack_id, "records" => [], "createdAt" => item["createdAt"],
            "origin" => { "listId" => item["id"] },
          }
          list = base.except("levels", "writing")
          list["name"] = item["name"]
          list["levels"] = item["levels"].dup if item["levels"].is_a?(Array) && !item["levels"].empty?
          list["writing"] = item["writing"].dup if item["writing"].is_a?(Hash)
          list["words"] = item["words"].map(&:dup)
          list
        end
      end
      by_id = packs.to_h { |pack| [pack["id"], pack] }
      placed = Set.new
      lists = library.lists.flat_map do |list|
        pack = by_id[list["packId"]]
        next [] if pack.nil?

        id = pack["id"]
        next [list] unless synced.key?(id)
        next [] unless placed.add?(id)

        synced[id]
      end
      Library.new(packs, lists)
    end
  end
end
