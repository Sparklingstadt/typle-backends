# A practice record (typle_records, one row per record of a list). at is written as the text PostgreSQL reads
# (RecordStore.timestamp), and data as the JSON the record was written to once.
class PracticeRecord < ApplicationRecord
  self.table_name = "typle_records"
  self.primary_key = %w[owner_id list_id record_id]

  attribute :data, Typle::JsonType.new
  attribute :at, :string

  CHANGED = Arel.sql(<<~SQL.squish)
    position = EXCLUDED.position, at = EXCLUDED.at, data = EXCLUDED.data
    WHERE (typle_records.position, typle_records.at, typle_records.data) IS DISTINCT FROM (EXCLUDED.position, EXCLUDED.at, EXCLUDED.data)
  SQL

  # Deletes the rows of owner but those of keys ([list_id, record_id] pairs).
  def self.delete_all_but(owner, keys)
    encoder = PG::TextEncoder::Array.new
    where(owner_id: owner).where(<<~SQL.squish, encoder.encode(keys.map(&:first)), encoder.encode(keys.map(&:last))).delete_all
      NOT EXISTS (SELECT 1 FROM unnest(?::text[], ?::text[]) AS k(list_id, record_id)
        WHERE k.list_id = typle_records.list_id AND k.record_id = typle_records.record_id)
    SQL
  end

  # Writes rows, changing a saved row only where it differs.
  def self.write_all(rows)
    upsert_all(rows, unique_by: %i[owner_id list_id record_id], on_duplicate: CHANGED, returning: false, record_timestamps: false)
  end
end
