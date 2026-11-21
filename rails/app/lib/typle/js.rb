module Typle
  # Strings and numbers as JavaScript treats them (kotlin/src/main/kotlin/typle/lib/JsText.kt and Json.kt): lengths in
  # UTF-16 units, trim() with ECMAScript's white space, Number.isSafeInteger, and Go's %g of src/server/records.ts.
  module Js
    SPACE = "[\\t\\n\\v\\f\\r \\u00a0\\u1680\\u2000-\\u200a\\u2028\\u2029\\u202f\\u205f\\u3000\\ufeff]"
    LEADING = /\A#{SPACE}+/o
    TRAILING = /#{SPACE}+\z/o
    SPACE_CHAR = /\A#{SPACE}\z/o
    MAX_SAFE = 9_007_199_254_740_991

    module_function

    # A header's value as Node reads it: each byte one character (Latin-1), whatever bytes Puma received.
    def header(value) = value&.b&.force_encoding(Encoding::ISO_8859_1)&.encode(Encoding::UTF_8)

    def space?(char) = SPACE_CHAR.match?(char)

    def trim(text) = text.sub(LEADING, "").sub(TRAILING, "")

    def trim_end(text) = text.sub(TRAILING, "")

    # String.prototype.toLocaleLowerCase in Node's default locale.
    def lower(text) = text.downcase

    # String.prototype.length: UTF-16 code units.
    def length(text)
      return text.length if text.ascii_only?

      text.each_codepoint.sum { |code| code > 0xFFFF ? 2 : 1 }
    end

    # String.prototype.slice(0, end) without splitting a surrogate pair (what fit() of workspaceDocument.ts keeps).
    def slice_to(text, stop)
      stop = [length(text) + stop, 0].max if stop.negative?
      units = 0
      kept = +""
      text.each_char do |char|
        size = char.ord > 0xFFFF ? 2 : 1
        break if units + size > stop

        units += size
        kept << char
      end
      kept
    end

    # Buffer.byteLength(text): its UTF-8 bytes.
    def utf8_bytesize(text) = text.bytesize

    def finite_number?(value) = value.is_a?(Integer) || (value.is_a?(Float) && value.finite?)

    def integer?(value) = value.is_a?(Integer) || (value.is_a?(Float) && value.finite? && value == value.floor)

    def safe_integer?(value) = integer?(value) && value.abs <= MAX_SAFE

    # The shortest digits of a finite, positive magnitude and n, with magnitude = 0.digits × 10^n (ECMAScript's k, n).
    # Ruby's Float#to_s writes the shortest decimal that reads back as the same double.
    def shortest_digits(magnitude)
      mantissa, exponent = magnitude.to_f.to_s.split("e")
      whole, fraction = mantissa.split(".")
      digits = whole + fraction.to_s
      n = whole.length + exponent.to_i
      while digits.length > 1 && digits.start_with?("0")
        digits = digits[1..]
        n -= 1
      end
      digits = digits.sub(/0+\z/, "")
      digits = "0" if digits.empty?
      [digits, n]
    end

    # Number.prototype.toString of a finite number (JSON.stringify writes numbers so; -0 is "0").
    def number(value)
      return value.to_s if value.is_a?(Integer)
      return "0" if value.zero?
      return "-#{number(-value)}" if value.negative?

      digits, n = shortest_digits(value)
      k = digits.length
      if n.between?(k, 21) then digits + ("0" * (n - k))
      elsif n.between?(1, 21) then "#{digits[0, n]}.#{digits[n..]}"
      elsif n.between?(-5, 0) then "0.#{"0" * -n}#{digits}"
      else
        e = n - 1
        mantissa = k == 1 ? digits : "#{digits[0]}.#{digits[1..]}"
        "#{mantissa}e#{e.negative? ? "-" : "+"}#{e.abs}"
      end
    end

    # value as Go's strconv.FormatFloat(value, 'g', -1, 64) writes it (goFloat of src/server/records.ts).
    def go_float(value)
      value = value.to_f
      return (1.0 / value).negative? ? "-0" : "0" if value.zero?
      return "-#{go_float(-value)}" if value.negative?

      digits, n = shortest_digits(value)
      exponent = n - 1
      if exponent < -4 || exponent >= 6
        mantissa = digits.length == 1 ? digits : "#{digits[0]}.#{digits[1..]}"
        return "#{mantissa}e#{exponent.negative? ? "-" : "+"}#{exponent.abs.to_s.rjust(2, "0")}"
      end
      number(value)
    end
  end
end
