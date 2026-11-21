module Typle
  # decodeDocument of src/lib/workspaceDocument.ts: packs missing or nil means none are saved.
  module WorkspaceDocuments
    module_function

    def decode(lists, packs, devices, interests)
      decoded_lists = WordLists.decode_lists(lists)
      decoded_devices = Devices.decode_devices(devices.nil? || devices.equal?(Value::UNDEFINED) ? [] : devices)
      decoded_interests = Interests.decode_interests(interests)
      decoded_packs = packs.nil? || packs.equal?(Value::UNDEFINED) ? nil : WordPacks.decode_packs(packs)
      library = WordPacks.normalize_library(decoded_lists, decoded_packs)
      WorkspaceDocument.new(library.packs, library.lists, decoded_devices, decoded_interests)
    end
  end
end
