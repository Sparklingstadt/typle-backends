module Typle
  # src/lib/records.ts: the checks and changes of a list's practice records.
  module Records
    RECORD_ID = /\A[A-Za-z0-9_-]+\z/
    Added = Struct.new(:lists, :record, :added)

    module_function

    def missed_words?(value, limit = Limits::MAX_MISSED_WORDS)
      value.is_a?(Array) && value.size <= limit && value.all? do |item|
        word = Value.prop(item, "word")
        count = Value.prop(item, "count")
        Value.object_like?(item) && (!item.is_a?(Hash) || item.keys.all? { |key| %w[word count].include?(key) }) &&
          word.is_a?(String) && !word.empty? && Js.length(word) <= Limits::MAX_WORD_TEXT &&
          Js.safe_integer?(count) && count >= 1
      end
    end

    def weak_review?(value)
      return false unless value.is_a?(Hash)

      date = Value.prop(value, "date")
      value.keys.all? { |key| %w[date missed].include?(key) } && date.is_a?(String) && JsDate.parse(date).finite? &&
        (!value.key?("missed") || missed_words?(value["missed"], Limits::MAX_WEAK_REVIEW_MISSED))
    end

    def counted?(value, min) = Js.finite_number?(value) && Js.safe_integer?(value) && value >= min

    def set?(input, key) = !Value.prop(input, key).equal?(Value::UNDEFINED)

    # Adds a run to a list (addRecord). A run with the id of one the list already has is not added again: that saved
    # record is returned with added false.
    def add_record(lists, devices, list_id, input)
      list = lists.find { |item| item["id"] == list_id } or raise AppError, "recordListDeleted"
      date_text = Value.prop(input, "date")
      time = date_text.is_a?(String) ? JsDate.parse(date_text) : Float::NAN
      year = JsDate.utc_year(time)
      run = Value.prop(input, "time")
      accuracy = Value.prop(input, "accuracy")
      mistakes = Value.prop(input, "mistakes")
      word_count = Value.prop(input, "wordCount")
      mode = Value.prop(input, "mode")
      id = Value.prop(input, "id")
      missed = Value.prop(input, "missed")
      if !Js.finite_number?(run) || run.negative? || !Js.finite_number?(accuracy) || accuracy.negative? || accuracy > 100 ||
         !(year >= 0 && year <= 9999) || !counted?(mistakes, 0) || (set?(input, "wordCount") && !counted?(word_count, 1)) ||
         (set?(input, "mode") && mode != "recall" && mode != "writing") || !id.is_a?(String) ||
         Js.length(id) > Limits::MAX_RECORD_ID || !RECORD_ID.match?(id) ||
         (set?(input, "missed") && !missed.nil? && !missed_words?(missed))
        raise AppError, "invalidRecord"
      end

      records = list["records"]
      saved = records.find { |record| Value.prop(record, "id") == id }
      return Added.new(lists, saved, false) if saved
      raise AppError, "recordLimit" if records.size >= Limits::MAX_RECORDS

      device = nil
      if set?(input, "device")
        sent = input["device"]
        raise AppError, "invalidRecord" if set?(input, "deviceId") || !Devices.device?(sent)

        device = { "id" => sent["id"], "name" => sent["name"], "layout" => sent["layout"] }
      elsif set?(input, "deviceId")
        device_id = input["deviceId"]
        device = if device_id == Devices::DEFAULT_DEVICE_ID
                   Devices.default_device(Catalog.default_device_name)
                 else
                   devices.find { |item| Value.prop(item, "id") == device_id }
                 end
        raise AppError, "deviceNotFound" if device.nil?
      end
      record = {
        "id" => id, "time" => run, "date" => JsDate.to_iso(time), "mistakes" => mistakes, "accuracy" => accuracy,
        "wordCount" => set?(input, "wordCount") ? word_count : list["words"].size,
      }
      record["mode"] = mode if set?(input, "mode")
      record["device"] = device.dup if device
      if missed.is_a?(Array) && !missed.empty?
        record["missed"] = missed.map { |item| { "word" => item["word"], "count" => item["count"] } }
      end
      next_lists = lists.map { |item| item.equal?(list) ? item.merge("records" => records + [record]) : item }
      Added.new(next_lists, record, true)
    end

    # Deletes a run from a list (deleteRecord); nil when it, or its list, is already gone.
    def delete_record(lists, list_id, record_id)
      list = lists.find { |item| item["id"] == list_id }
      records = list&.dig("records")
      return nil if record_id.empty? || records.nil? || records.none? { |record| Value.prop(record, "id") == record_id }

      lists.map do |item|
        item.equal?(list) ? item.merge("records" => records.reject { |record| Value.prop(record, "id") == record_id }) : item
      end
    end
  end
end
