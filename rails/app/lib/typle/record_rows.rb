require "digest"

module Typle
  # The rows of typle_records (src/server/records.ts): one per practice record, put back in their lists on reads.
  module RecordRows
    # A record as typle_records keeps it. at is milliseconds since the epoch; data is the record as JSON.
    Row = Struct.new(:list_id, :record_id, :position, :at, :data)

    module_function

    # The id a record without one (or with an earlier record's id) is given: the same record always gets the same id.
    def legacy_id(list_id, position, date, time)
      "legacy-" + Digest::SHA256.hexdigest("#{list_id}\0#{position}\0#{date}\0#{Js.go_float(time)}")[0, 20]
    end

    # The records of the lists as rows, every record given an id that keeps it apart from the rest of its list, in place
    # (as recordRows does, so the size of a save counts the ids too).
    def rows(lists)
      lists.flat_map do |list|
        seen = Set.new
        list_id = list["id"]
        list["records"].each_with_index.map do |record, position|
          id = record["id"]
          if !id.is_a?(String) || id.empty? || seen.include?(id)
            record["id"] = legacy_id(list_id, position, record["date"], record["time"])
          end
          seen << record["id"]
          Row.new(list_id, record["id"], position, JsDate.parse(record["date"]), JsonValue.generate(record))
        end
      end
    end

    # The lists with the stored records put back in the lists that hold them (withRecords).
    def with_records(lists, stored)
      lists.map do |item|
        raise TypeError, "Cannot read properties of null (reading 'id')" if item.nil?

        id = Value.prop(item, "id")
        moved = id.is_a?(String) ? stored.fetch(id, []) : []
        next item if moved.empty?

        moved_ids = moved.to_set { |record| record_id_of(record) }
        own = Value.prop(item, "records")
        own = [] unless own.is_a?(Array)
        kept = own.select { |record| (rid = record_id_of(record)) == "" || !moved_ids.include?(rid) }
        (item.is_a?(Hash) ? item : {}).merge("records" => kept + moved)
      end
    end

    def record_id_of(record)
      id = Value.prop(record, "id")
      id.is_a?(String) ? id : ""
    end
  end
end
