module Typle
  # An error named by a key of the app's `errors` messages (src/lib/appError.ts).
  class AppError < StandardError
    attr_reader :key

    def initialize(key)
      @key = key
      super(key)
    end
  end
end
