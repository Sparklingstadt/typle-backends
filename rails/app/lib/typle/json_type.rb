module Typle
  # A jsonb column read and written as JSON.parse and JSON.stringify do (JsonValue), not as ActiveSupport::JSON does.
  # A Raw value is JSON text already written (a record's data, written once for its row).
  class JsonType < ActiveModel::Type::Value
    Raw = Struct.new(:text)

    def type = :jsonb

    def deserialize(value) = value.is_a?(String) ? JsonValue.parse(value) : value

    def serialize(value)
      case value
      when nil then nil
      when Raw then value.text
      else JsonValue.generate(value)
      end
    end
  end
end
