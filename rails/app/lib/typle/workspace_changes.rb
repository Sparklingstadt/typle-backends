module Typle
  # The change a PUT /api/workspace body asks for, and what a save writes (readWorkspaceChange and encodeChange of
  # src/server/workspace.ts).
  module WorkspaceChanges
    MAX_LISTS_BYTES = 896 << 10
    Encoded = Struct.new(:devices, :interests, :library)
    EncodedLibrary = Struct.new(:lists, :packs, :rows, :own, :followed, :size)
    INVALID = [:refused, "invalidWorkspace"].freeze

    module_function

    # Whether value holds a string (or key) jsonb cannot keep: one with U+0000 or one that is not valid UTF-8.
    def unstorable?(value)
      case value
      when String then !value.valid_encoding? || value.include?("\0")
      when Array then value.any? { |item| unstorable?(item) }
      when Hash then value.any? { |key, item| unstorable?(key) || unstorable?(item) }
      else false
      end
    end

    # [:ok, revision, change], or [:refused, key] (invalidWorkspace or levelsConflict).
    def read(fields)
      return INVALID if %w[lists packs devices].any? { |name| fields.key?(name) && fields[name].nil? }
      return INVALID if %w[lists packs devices interests].any? { |name| fields.key?(name) && unstorable?(fields[name]) }

      interests = Value.prop(fields, "interests")
      rest = fields.except("accountId", "interests")
      several = {}
      lists = Value.prop(rest, "lists")
      # A save may hold up to MAX_SAVED_LEVELS levels of a list sent with its packs, which the checks of one level let pass.
      if lists.is_a?(Array) && rest.key?("packs")
        lists = lists.each_with_index.map do |item, index|
          levels = Value.prop(item, "levels")
          next item if !levels.is_a?(Array) || levels.size <= 1 || levels.size > Limits::MAX_SAVED_LEVELS ||
                       levels.any? { |level| !level.is_a?(String) || Js.length(level) > Limits::MAX_LEVEL }

          several[index] = levels
          item.merge("levels" => ["\0several-#{index}"])
        end
      end
      body = rest.dup
      body["lists"] = lists unless lists.equal?(Value::UNDEFINED)
      body["interests"] = interests unless interests.equal?(Value::UNDEFINED) || interests.nil?
      begin
        revision, change = WorkspaceRequest.validate(body)
      rescue SystemStackError
        return INVALID
      rescue AppError => e
        return e.key == "levelsConflict" ? [:refused, "levelsConflict"] : INVALID
      rescue StandardError
        return INVALID
      end
      if change.lists && !several.empty?
        change.lists = change.lists.each_with_index.map { |list, index| several.key?(index) ? list.merge("levels" => several[index]) : list }
      end
      change.interests = [] if fields.key?("interests") && interests.nil?
      [:ok, revision, change]
    end

    # encodeChange: the repaired lists and packs a save writes, without followed words and records, and their rows.
    def encode(change)
      return Encoded.new(change.devices, change.interests, nil) if change.lists.nil?

      repaired = Catalog.followed(WordPacks.repair_library(Library.new(change.packs || [], change.lists)))
      rows = RecordRows.rows(repaired.lists)
      slim = FollowedPacks.without_followed_words(repaired)
      # A change the app could not read back is never written.
      begin
        document = WorkspaceDocuments.decode(slim.lists, slim.packs, change.devices || [], change.interests.nil? ? Value::UNDEFINED : change.interests)
        Catalog.followed(document.library)
      rescue StandardError
        raise AppError, "invalidWorkspace"
      end
      size = Js.utf8_bytesize(JsonValue.generate(slim.lists)) + Js.utf8_bytesize(JsonValue.generate(slim.packs))
      Encoded.new(change.devices, change.interests, EncodedLibrary.new(
        slim.lists.map { |list| list.merge("records" => []) }, slim.packs, rows,
        WordPacks.own_list_count(repaired), WordPacks.followed_pack_count(repaired.packs), size,
      ))
    end
  end
end
