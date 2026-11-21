# The port of src/lib and src/server that the compared endpoints need (the Kotlin backend's kotlin/ is the same port).
# Values are JSON as JsonValue reads it: Hashes with String keys (in their order), Arrays, Strings, Integers, Floats,
# true, false and nil. A key that is not there is JavaScript's undefined (UNDEFINED, which prop returns).
module Typle
  module Value
    UNDEFINED = Object.new.freeze

    module_function

    # value[key] as JavaScript reads a property: UNDEFINED when value is not an object or has no such key.
    def prop(value, key) = value.is_a?(Hash) && value.key?(key) ? value[key] : UNDEFINED

    def object_like?(value) = value.is_a?(Hash) || value.is_a?(Array)

    # Whether JavaScript takes value as true in a condition.
    def truthy?(value)
      !(value.nil? || value.equal?(UNDEFINED) || value == false || value == "" ||
        (value.is_a?(Numeric) && (value.zero? || (value.is_a?(Float) && value.nan?))))
    end

    def deep_copy(value)
      case value
      when Hash then value.to_h { |key, item| [key, deep_copy(item)] }
      when Array then value.map { |item| deep_copy(item) }
      else value
      end
    end
  end
end
