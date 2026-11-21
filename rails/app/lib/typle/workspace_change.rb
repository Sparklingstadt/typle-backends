module Typle
  # A change of a save; nil parts stay as they are.
  WorkspaceChange = Struct.new(:lists, :packs, :devices, :interests)
end
