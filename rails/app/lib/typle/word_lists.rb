module Typle
  # src/lib/wordLists.ts and the record checks of src/lib/records.ts that reading lists needs.
  module WordLists
    WRITING_LANGUAGES = %w[en ja zh-Hans zh-Hant ko fr de es ru cs].freeze
    LIST_NULLS = %w[writing tags category group levels packId].freeze
    RECORD_NULLS = %w[id mistakes accuracy wordCount device missed].freeze

    module_function

    def input_type?(value) = %w[command code word].include?(value)

    # A valid mark: an object whose only key is a known language.
    def writing_mark?(value)
      value.is_a?(Hash) && value.size == 1 && WRITING_LANGUAGES.include?(Value.prop(value, "language"))
    end

    def validate_list(list)
      raise AppError, "listNameRequired" if Js.trim(list["name"]).empty?

      words = list["words"]
      raise AppError, "wordsRequired" if words.empty?
      raise AppError, "wordFieldsRequired" if words.any? { |word| Js.trim(word["display"]).empty? || Js.trim(word["input"]).empty? }
      raise AppError, "savedDataCorrupted" if words.any? { |word| word.key?("input_type") && !input_type?(word["input_type"]) }
      raise AppError, "invalidWritingLanguage" if list.key?("writing") && !writing_mark?(list["writing"])
    end

    def drop_nulls(value, names)
      names.each { |name| value.delete(name) if value.key?(name) && value[name].nil? }
    end

    def present?(value, key) = !Value.prop(value, key).equal?(Value::UNDEFINED)

    def bad_record?(record)
      return true unless Value.truthy?(record)

      time = Value.prop(record, "time")
      date = Value.prop(record, "date")
      accuracy = Value.prop(record, "accuracy")
      mistakes = Value.prop(record, "mistakes")
      word_count = Value.prop(record, "wordCount")
      mode = Value.prop(record, "mode")
      !Js.finite_number?(time) || time.negative? || !date.is_a?(String) || !JsDate.parse(date).finite? ||
        (present?(record, "device") && !Devices.device?(record["device"])) ||
        (present?(record, "mode") && mode != "recall" && mode != "writing") ||
        (present?(record, "id") && !record["id"].is_a?(String)) ||
        (present?(record, "accuracy") && (!Js.finite_number?(accuracy) || accuracy.negative? || accuracy > 100)) ||
        (present?(record, "mistakes") && (!Js.integer?(mistakes) || mistakes.negative?)) ||
        (present?(record, "wordCount") && (!Js.integer?(word_count) || word_count < 1)) ||
        (present?(record, "missed") && !Records.missed_words?(record["missed"]))
    end

    def bad_word?(word)
      return true unless Value.truthy?(word)

      !Value.prop(word, "display").is_a?(String) || !Value.prop(word, "input").is_a?(String) ||
        !Value.prop(word, "annotation").is_a?(String) || (present?(word, "input_type") && !input_type?(word["input_type"]))
    end

    def bad_list?(list, ids)
      return true unless list.is_a?(Hash)

      id = list["id"]
      words = Value.prop(list, "words")
      records = Value.prop(list, "records")
      tags = Value.prop(list, "tags")
      levels = Value.prop(list, "levels")
      !id.is_a?(String) || id.empty? || ids.include?(id) || !Value.prop(list, "name").is_a?(String) ||
        !Value.prop(list, "createdAt").is_a?(String) || !words.is_a?(Array) || !records.is_a?(Array) ||
        (present?(list, "category") && !list["category"].is_a?(String)) ||
        (present?(list, "tags") && (!tags.is_a?(Array) || tags.any? { |tag| !tag.is_a?(String) })) ||
        (present?(list, "group") && !list["group"].is_a?(String)) ||
        (present?(list, "packId") && !list["packId"].is_a?(String)) ||
        (present?(list, "writing") && !writing_mark?(list["writing"])) ||
        (present?(list, "levels") && (!levels.is_a?(Array) || levels.any? { |level| !level.is_a?(String) })) ||
        words.any? { |word| bad_word?(word) } || records.any? { |record| bad_record?(record) }
    end

    # decodeLists(JSON.stringify({ version: 1, lists: value })): the lists checked, as copies.
    def decode_lists(value)
      raise AppError, "savedDataUnreadable" if value.equal?(Value::UNDEFINED)

      data = Value.deep_copy(value)
      raise AppError, "savedDataUnreadable" unless data.is_a?(Array)

      data.each do |list|
        next unless list.is_a?(Hash)

        drop_nulls(list, LIST_NULLS)
        records = list["records"]
        records.each { |record| drop_nulls(record, RECORD_NULLS) if record.is_a?(Hash) } if records.is_a?(Array)
      end
      ids = Set.new
      data.each do |list|
        raise AppError, "savedDataCorrupted" if bad_list?(list, ids)

        # A list of a followed pack (with an origin) is saved without its words, which the catalog gives back.
        if !list["words"].empty? || !Value.prop(Value.prop(list, "origin"), "listId").is_a?(String)
          validate_list(list)
        elsif Js.trim(list["name"]).empty?
          raise AppError, "listNameRequired"
        end
        ids << list["id"]
      end
      # Fields of link sharing are dropped, and an origin or weak review of another shape.
      data.map do |list|
        result = list.except("shared", "shareId", "origin", "weakReview")
        result["origin"] = list["origin"] if list.key?("origin") && Value.prop(list["origin"], "listId").is_a?(String)
        result["weakReview"] = list["weakReview"] if list.key?("weakReview") && Records.weak_review?(list["weakReview"])
        result
      end
    end
  end
end
