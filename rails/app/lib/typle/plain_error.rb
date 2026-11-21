module Typle
  # A plain Error of the TypeScript code (new Error(...)): not an AppError, so its caller answers it with its default.
  class PlainError < StandardError; end
end
