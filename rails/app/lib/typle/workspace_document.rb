module Typle
  # A workspace as the app reads it.
  WorkspaceDocument = Struct.new(:packs, :lists, :devices, :interests) do
    def library = Library.new(packs, lists)
  end
end
