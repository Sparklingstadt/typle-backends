module Typle
  # src/lib/interests.ts: the ids of the options, in the order they are shown and saved.
  module Interests
    OPTIONS = [
      ["language", %w[zh ru ko en es fr de cs]],
      ["programming", %w[go kotlin java typescript php ruby sql linux vim test node shell cloud]],
    ].freeze

    # The interests of a user who has saved none: every option (tests/fixtures/default-interests.json).
    DEFAULT_INTERESTS = OPTIONS.flat_map { |id, children| [id] + children.map { |child| "#{id}/#{child}" } }.freeze

    module_function

    def decode_interests(value)
      return DEFAULT_INTERESTS.dup if value.nil? || value.equal?(Value::UNDEFINED)
      if !value.is_a?(Array) || value.size > Limits::MAX_INTERESTS ||
         value.any? { |item| !item.is_a?(String) || item.empty? || Js.length(item) > Limits::MAX_INTEREST_LENGTH } ||
         value.uniq.size != value.size
        raise AppError, "savedDataCorrupted"
      end

      value.dup
    end
  end
end
