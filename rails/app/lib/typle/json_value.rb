module Typle
  # JSON as JSON.parse and JSON.stringify treat it: a repeated key keeps its last value, and a number is one Number, so
  # 1500.0 is 1500 (an Integer here, written "1500" as JSON.stringify writes it). Ruby's parser refuses a lone surrogate
  # in a string, which JSON.parse takes; every body that holds one is refused all the same (a string jsonb cannot keep).
  module JsonValue
    MAX_EXACT = 2**53

    module_function

    def parse(text)
      normalize(JSON.parse(text, allow_duplicate_key: true, allow_nan: false))
    end

    # JSON.stringify(value): a non-finite number is null, and a Float is written as Number#toString writes it.
    def generate(value)
      JSON.generate(js_numbers(value))
    end

    def js_numbers(value)
      case value
      when Hash then value.transform_values { |item| js_numbers(item) }
      when Array then value.map { |item| js_numbers(item) }
      when Float then value.finite? ? JSON::Fragment.new(Js.number(value)) : nil
      else value
      end
    end

    # value with every Float that is a whole number within 2^53 as an Integer, and every larger Integer as a Float.
    def normalize(value)
      case value
      when Hash then value.transform_values { |item| normalize(item) }
      when Array then value.map { |item| normalize(item) }
      when Float then value.finite? && value == value.floor && value.abs <= MAX_EXACT ? value.to_i : value
      when Integer then value.abs > MAX_EXACT ? value.to_f : value
      else value
      end
    end
  end
end
