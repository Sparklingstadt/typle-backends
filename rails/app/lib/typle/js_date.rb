module Typle
  # Dates as V8's Date.parse reads them and Date.prototype.toISOString writes them (JsDate of
  # kotlin/src/main/kotlin/typle/lib/JsText.kt). Times are milliseconds since the epoch, as Floats (NaN when invalid).
  # Without an offset a date and time is local time, which is UTC where the servers run.
  module JsDate
    MAX_TIME = 8.64e15
    DAY_MS = 86_400_000
    EPOCH_JD = Date.new(1970, 1, 1, Date::GREGORIAN).jd
    ISO = /\A([+-]\d{6}|\d{4})(?:-(\d{2})(?:-(\d{2}))?)?(?:T(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d+))?)?)?(Z|[+-]\d{2}:\d{2})?\z/
    NUMERIC = /\A\s*(\d{1,6})[-\/](\d{1,2})[-\/](\d{1,2})(?:[T ]+(\d{1,2}):(\d{2})(?::(\d{2})(?:\.(\d+))?)?)?\s*(Z|UTC|GMT|(?:(?:UTC|GMT)?[+-]\d{2}:?\d{2}))?\s*\z/i
    WORDED = /\A\s*(?:[A-Za-z]+,?\s+)?(?:(\d{1,2})\s+([A-Za-z]+)|([A-Za-z]+)\s+(\d{1,2}),?)\s+(\d{1,6})(?:\s+(\d{1,2}):(\d{2})(?::(\d{2}))?)?\s*((?:UTC|GMT|Z)?(?:[+-]\d{2}:?\d{2})?)\s*(?:\([^)]*\))?\s*\z/i
    MONTHS = %w[jan feb mar apr may jun jul aug sep oct nov dec].freeze
    OFFSET = /([+-])(\d{2}):?(\d{2})\z/

    module_function

    # Date.parse(text): milliseconds since the epoch, or NaN.
    def parse(text)
      match = ISO.match(text)
      match ? parse_iso(match) : parse_legacy(text)
    end

    # Date.prototype.toISOString of a valid time.
    def to_iso(time)
      raise ArgumentError, "Invalid time value" if time.nan?

      t = time.to_i
      days, in_day = t.divmod(DAY_MS)
      date = Date.jd(EPOCH_JD + days, Date::GREGORIAN)
      year = date.year
      year_text = year.between?(0, 9999) ? year.to_s.rjust(4, "0") : "#{year.negative? ? "-" : "+"}#{year.abs.to_s.rjust(6, "0")}"
      format("%s-%02d-%02dT%02d:%02d:%02d.%03dZ", year_text, date.month, date.day,
             in_day / 3_600_000, in_day / 60_000 % 60, in_day / 1000 % 60, in_day % 1000)
    end

    # getUTCFullYear of a time, NaN for NaN.
    def utc_year(time)
      return Float::NAN if time.nan?

      Date.jd(EPOCH_JD + time.to_i.div(DAY_MS), Date::GREGORIAN).year.to_f
    end

    # ECMAScript's MakeDay (with month overflow) and MakeDate, in UTC.
    def make_time(year, month, day, hour, minute, second, ms)
      y = year + month.div(12)
      return Float::NAN if y.abs > 400_000

      days = Date.new(y, month % 12 + 1, 1, Date::GREGORIAN).jd - EPOCH_JD + day - 1
      days * DAY_MS.to_f + (((hour * 60) + minute) * 60 + second) * 1000.0 + ms
    end

    def clip(time) = time.nan? || time.abs > MAX_TIME ? Float::NAN : time

    def millis(text) = text.nil? ? 0 : "#{text}00"[0, 3].to_i

    def parse_iso(match)
      year_text, month, day, hour, minute, second, fraction, zone = match.captures
      return Float::NAN if year_text == "-000000"

      month = month ? month.to_i : 1
      day = day ? day.to_i : 1
      has_time = !hour.nil?
      hour = hour.to_i
      minute = minute.to_i
      second = second.to_i
      ms = millis(fraction)
      return Float::NAN if !month.between?(1, 12) || !day.between?(1, 31) || minute > 59 || second > 59
      return Float::NAN if hour > 24 || (hour == 24 && (minute != 0 || second != 0 || ms != 0))
      return Float::NAN if !has_time && zone

      time = make_time(year_text.to_i, month - 1, day, hour, minute, second, ms)
      if zone && zone != "Z"
        sign = zone.start_with?("-") ? -1 : 1
        off_hour = zone[1, 2].to_i
        off_minute = zone[4, 2].to_i
        return Float::NAN if off_hour > 23 || off_minute > 59

        time -= sign * ((off_hour * 60) + off_minute) * 60_000.0
      end
      clip(time)
    end

    def parse_legacy(text)
      if (m = NUMERIC.match(text))
        return legacy_time(m[1].to_i, m[2].to_i, m[3].to_i, m[4], m[5], m[6], millis(m[7]), m[8].to_s)
      end
      if (m = WORDED.match(text))
        day_text, month_text = m[1] ? [m[1], m[2]] : [m[4], m[3]]
        month = MONTHS.index(month_text.downcase[0, 3])
        return Float::NAN if month.nil? || month_text.length < 3

        return legacy_time(m[5].to_i, month + 1, day_text.to_i, m[6], m[7], m[8], 0, m[9].to_s)
      end
      Float::NAN
    end

    def legacy_time(year, month, day, hour, minute, second, ms, zone)
      return Float::NAN unless month.between?(1, 12) && day.between?(1, 31)

      h = hour.to_i
      mi = minute.to_i
      s = second.to_i
      return Float::NAN if h > 24 || mi > 59 || s > 59

      time = make_time(year, month - 1, day, h, mi, s, ms)
      if (offset = OFFSET.match(zone))
        sign = offset[1] == "-" ? -1 : 1
        time -= sign * ((offset[2].to_i * 60) + offset[3].to_i) * 60_000.0
      end
      clip(time)
    end
  end
end
