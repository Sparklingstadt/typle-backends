module Typle
  # src/lib/wordPacks.ts, src/lib/workspaceDocument.ts (unique, fit) and the counts of src/lib/listOverview.ts.
  module WordPacks
    EDGE = /\A(?:#{Js::SPACE}|・)+|(?:#{Js::SPACE}|・)+\z/o
    LABELS = %w[group category tags].freeze

    module_function

    def name_key(name) = Js.lower(Js.trim(name))

    # unique of workspaceDocument.ts: wanted, or its first variant whose lowercase is not taken yet.
    def unique(taken, wanted, variant)
      value = wanted
      count = 2
      while taken.include?(Js.lower(value))
        value = variant.(count)
        count += 1
      end
      taken << Js.lower(value)
      value
    end

    # fit of workspaceDocument.ts: name cut to leave room for suffix, without splitting a surrogate pair.
    def fit(name, suffix, limit = Limits::MAX_NAME)
      Js.trim_end(Js.slice_to(name, limit - Js.length(suffix))) + suffix
    end

    def numbered(name) = ->(count) { fit(name, "（#{count}）") }

    def level_path(list)
      levels = list["levels"]
      levels.is_a?(Array) && !levels.empty? ? levels : [list["name"]]
    end

    def paths_overlap?(a, b)
      short, long = a.size <= b.size ? [a, b] : [b, a]
      short.each_index.all? { |i| short[i] == long[i] }
    end

    def bare(level) = level.gsub(EDGE, "")

    def join_levels(levels)
      joined = bare(fit(levels.map { |level| bare(level) }.reject(&:empty?).join("・"), "", Limits::MAX_LEVEL))
      joined.empty? ? nil : joined
    end

    def collapse_levels(list)
      levels = list["levels"]
      return list if levels.nil? || levels.size <= 1

      joined = join_levels(levels)
      joined ? list.merge("levels" => [joined]) : list.except("levels")
    end

    def separate_levels(library)
      lists = library.lists.map { |list| collapse_levels(list) }
      names = Hash.new { |hash, key| hash[key] = Set.new }
      lists.each { |list| names[list["packId"]] << name_key(list["name"]) }
      accepted = Hash.new { |hash, key| hash[key] = [] }
      Library.new(library.packs, lists.map do |list|
        pack_id = list["packId"]
        paths = accepted[pack_id]
        taken = names[pack_id]
        has_levels = list["levels"].is_a?(Array) && !list["levels"].empty?
        base = level_path(list)
        path = base.dup
        name = list["name"]
        name_taken = -> { !has_levels && name_key(path[0]) != name_key(name) && taken.include?(name_key(path[0])) }
        count = 2
        while paths.any? { |other| paths_overlap?(other, path) } || name_taken.()
          path[0] = fit(base[0], "（#{count}）", has_levels ? Limits::MAX_LEVEL : Limits::MAX_LIST_NAME)
          count += 1
        end
        paths << path
        if path[0] == base[0] then list
        elsif has_levels then list.merge("levels" => path)
        else
          taken << name_key(path[0])
          list.merge("name" => path[0])
        end
      end)
    end

    # A pack with description, tags and origin.followed saved as null left out (withoutNulls).
    def without_nulls(pack)
      return pack unless pack.is_a?(Hash)

      origin = Value.prop(pack, "origin")
      clean_origin = origin.is_a?(Hash) && origin.key?("followed") && origin["followed"].nil? ? origin.except("followed") : origin
      result = pack.except("description", "tags", "origin")
      result["description"] = pack["description"] unless pack["description"].nil?
      result["tags"] = pack["tags"] unless pack["tags"].nil?
      result["origin"] = clean_origin if pack.key?("origin")
      result
    end

    def decode_packs(value)
      raise AppError, "savedDataCorrupted" unless value.is_a?(Array)

      packs = value.map { |pack| without_nulls(pack) }
      ids = Set.new
      packs.each do |pack|
        if !Value.truthy?(pack) || !pack.is_a?(Hash) || !pack["id"].is_a?(String) || pack["id"].empty? || ids.include?(pack["id"]) ||
           !Value.prop(pack, "name").is_a?(String) || !Value.prop(pack, "createdAt").is_a?(String) ||
           (pack.key?("description") && !pack["description"].is_a?(String)) ||
           (pack.key?("category") && !pack["category"].is_a?(String)) ||
           (pack.key?("tags") && (!pack["tags"].is_a?(Array) || pack["tags"].any? { |tag| !tag.is_a?(String) })) ||
           (pack.key?("origin") && bad_origin?(pack["origin"]))
          raise AppError, "savedDataCorrupted"
        end

        ids << pack["id"]
      end
      packs
    end

    def bad_origin?(origin)
      followed = Value.prop(origin, "followed")
      !Value.truthy?(origin) || !Value.prop(origin, "catalogId").is_a?(String) ||
        !Value.prop(origin, "version").is_a?(Numeric) || (!followed.equal?(Value::UNDEFINED) && followed != true)
    end

    # normalizeLabels of wordLists.ts: category and tags trimmed, empty and repeated tags dropped, absent when none.
    def normalize_labels(category, tags)
      name = category.is_a?(String) ? Js.trim(category) : ""
      seen = Set.new
      kept = []
      (tags || []).each do |tag|
        value = Js.trim(tag)
        next if value.empty? || seen.include?(Js.lower(value))

        seen << Js.lower(value)
        kept << value
      end
      result = {}
      result["category"] = name unless name.empty?
      result["tags"] = kept unless kept.empty?
      result
    end

    def strings(value) = value.is_a?(Array) ? value : nil

    def without_labels(list) = list.except(*LABELS)

    def labels_of(category, tags)
      labels = normalize_labels(category, tags)
      labels["tags"] = labels["tags"].first(Limits::MAX_TAGS) if labels["tags"]
      labels
    end

    def group_of(list) = list["group"].is_a?(String) ? Js.trim(list["group"]) : nil

    # Makes packs of lists written with groups (convertLibrary), as the official lists are made into packs.
    def convert_library(lists)
      packs = []
      pack_of = {}
      names = Set.new
      pack_ids = lists.map do |list|
        group = group_of(list)
        key = group && !group.empty? ? "group:#{group}" : "list:#{list["id"]}"
        next pack_of[key]["id"] if pack_of.key?(key)

        members = group && !group.empty? ? lists.select { |other| group_of(other) == group } : [list]
        oldest = members.reduce { |a, b| JsDate.parse(b["createdAt"]) < JsDate.parse(a["createdAt"]) ? b : a }
        wanted = group && !group.empty? ? group : Js.trim(list["name"])
        pack = { "id" => "pack-#{list["id"]}", "name" => unique(names, wanted, numbered(wanted)) }
        pack.merge!(labels_of(list["category"], members.flat_map { |member| strings(member["tags"]) || [] }))
        pack["createdAt"] = oldest["createdAt"]
        pack_of[key] = pack
        packs << pack
        pack["id"]
      end
      separate_levels(Library.new(packs, lists.each_with_index.map { |list, i| without_labels(list).merge("packId" => pack_ids[i]) }))
    end

    # Keeps the rules of ownership for data an older app saved: every list in one pack, no empty packs, no repeated names.
    def repair_library(library)
      packs = library.packs.map(&:dup)
      by_id = packs.to_h { |pack| [pack["id"], pack] }
      placed = library.lists.map do |list|
        pack_id = list["packId"]
        next list if pack_id.is_a?(String) && by_id.key?(pack_id)

        id = "pack-#{list["id"]}"
        unless by_id.key?(id)
          pack = { "id" => id, "name" => Js.trim(list["name"]), "createdAt" => list["createdAt"] }
          by_id[id] = pack
          packs << pack
        end
        list.merge("packId" => id)
      end
      # Labels an older app left on a list move to the pack only while the pack has none.
      lists = placed.map do |list|
        pack = by_id[list["packId"]]
        pack_tags = pack["tags"]
        if !Value.truthy?(pack["category"]) && (pack_tags.nil? || pack_tags.empty?)
          pack.merge!(normalize_labels(list["category"], strings(list["tags"])))
        end
        without_labels(list)
      end
      used = lists.to_set { |list| list["packId"] }
      names = Set.new
      kept = packs.select { |pack| used.include?(pack["id"]) }.map do |pack|
        name = Js.trim(pack["name"])
        pack.merge("name" => unique(names, name, numbered(name)))
      end
      list_names = Hash.new { |hash, key| hash[key] = Set.new }
      separate_levels(Library.new(kept, lists.map do |list|
        name = Js.trim(list["name"])
        list.merge("name" => unique(list_names[list["packId"]], name, numbered(name)))
      end))
    end

    def normalize_library(lists, packs) = repair_library(Library.new(packs || [], lists))

    # Refuses a library that is not in the saved shape (PlainError), or whose levels a save may not have (AppError).
    def validate_library(library)
      ids = Set.new
      names = Set.new
      library.packs.each do |pack|
        name = name_key(pack["name"])
        id = pack["id"]
        tags = strings(pack["tags"])
        if id.empty? || ids.include?(id) || name.empty? || Js.length(pack["name"]) > Limits::MAX_PACK_NAME || names.include?(name) ||
           Js.length(pack["description"] || "") > Limits::MAX_PACK_DESCRIPTION ||
           Js.length(pack["category"] || "") > Limits::MAX_CATEGORY ||
           (tags&.size || 0) > Limits::MAX_TAGS || tags&.any? { |tag| Js.length(tag) > Limits::MAX_TAG }
          raise PlainError, "Invalid packs"
        end

        ids << id
        names << name
      end
      list_names = Hash.new { |hash, key| hash[key] = Set.new }
      library.lists.each do |list|
        pack_id = list["packId"]
        taken = list_names[pack_id || ""]
        name = name_key(list["name"])
        if pack_id.nil? || pack_id.empty? || !ids.include?(pack_id) || taken.include?(name) || LABELS.any? { |key| list.key?(key) }
          raise PlainError, "Invalid packs"
        end

        taken << name
      end
      raise PlainError, "Invalid packs" if ids.size != list_names.size

      paths = Hash.new { |hash, key| hash[key] = [] }
      library.lists.each do |list|
        raise AppError, "tooManyLevels" if (list["levels"]&.size || 0) > Limits::MAX_LEVELS

        mine = paths[list["packId"]]
        raise AppError, "levelsConflict" if mine.any? { |path| paths_overlap?(path, level_path(list)) }

        mine << level_path(list)
      end
    end

    def followed_ids(packs) = packs.select { |pack| Value.prop(pack["origin"], "followed") == true }.to_set { |pack| pack["id"] }

    def own_list_count(library)
      followed = followed_ids(library.packs)
      library.lists.count { |list| !followed.include?(list["packId"]) }
    end

    def followed_pack_count(packs) = followed_ids(packs).size
  end
end
