module Typle
  # src/lib/workspaceRequest.ts.
  module WorkspaceRequest
    module_function

    def too_large?(list)
      words = list["words"]
      tags = list["tags"]
      levels = list["levels"]
      words.size > Limits::MAX_WORDS || list["records"].size > Limits::MAX_RECORDS ||
        Js.length(list["name"]) > Limits::MAX_LIST_NAME || Js.length(list["category"] || "") > Limits::MAX_CATEGORY ||
        (tags&.size || 0) > Limits::MAX_TAGS || tags&.any? { |tag| Js.length(tag) > Limits::MAX_TAG } ||
        Js.length(list["group"] || "") > Limits::MAX_GROUP ||
        (levels&.size || 0) > Limits::MAX_SAVED_LEVELS || levels&.any? { |level| Js.length(level) > Limits::MAX_LEVEL } ||
        words.any? do |word|
          Js.length(word["input"]) > Limits::MAX_WORD_TEXT || Js.length(word["display"]) > Limits::MAX_WORD_TEXT ||
            Js.length(word["annotation"]) > Limits::MAX_ANNOTATION
        end
    end

    # validateLists: the lists read as saved data, within the limits of a save (PlainError past them).
    def validate_lists(value)
      lists = WordLists.decode_lists(value)
      raise PlainError, "Workspace too large" if lists.any? { |list| too_large?(list) }

      lists
    end

    # validateWorkspaceRequest: the revision and the change of a body.
    def validate(body)
      revision = Value.prop(body, "revision")
      raise PlainError, "Invalid revision" unless Js.safe_integer?(revision) && revision >= 0
      raise PlainError, "Invalid request" if body.key?("profileId") || body.key?("profiles")
      raise PlainError, "Invalid request" if body.key?("packs") && !body.key?("lists")

      lists = body.key?("lists") ? validate_lists(body["lists"]) : nil
      packs = body.key?("packs") ? WordPacks.decode_packs(body["packs"]) : nil
      WordPacks.validate_library(Library.new(packs, lists)) if lists && packs
      devices = body.key?("devices") ? Devices.decode_devices(body["devices"]) : nil
      interests = body.key?("interests") ? Interests.decode_interests(body["interests"]) : nil
      [revision.to_i, WorkspaceChange.new(lists, packs, devices, interests)]
    end
  end
end
