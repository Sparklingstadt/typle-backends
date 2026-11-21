# A user's workspace (typle_workspaces, one row per owner): lists, packs, devices and interests as jsonb, and the
# revision every save moves on. The table belongs to Next.js (db/ of the repository).
class Workspace < ApplicationRecord
  self.table_name = "typle_workspaces"
  self.primary_key = "owner_id"

  attribute :lists, Typle::JsonType.new
  attribute :packs, Typle::JsonType.new
  attribute :devices, Typle::JsonType.new
  attribute :interests, Typle::JsonType.new

  # The records of a split row, as [list_id, data] pairs in their order; nil when the records are still in the lists.
  RECORDS = Arel.sql(<<~SQL.squish)
    CASE WHEN records_split THEN (SELECT COALESCE(jsonb_agg(jsonb_build_array(r.list_id, r.data) ORDER BY r.list_id, r.position), '[]'::jsonb)
      FROM typle_records r WHERE r.owner_id = typle_workspaces.owner_id)::text END
  SQL

  COUNTS = <<~SQL.squish
    WITH followed AS (
      SELECT p->>'id' AS id FROM typle_workspaces, jsonb_array_elements(COALESCE(packs, '[]'::jsonb)) AS p
      WHERE owner_id = ? AND p->'origin'->>'followed' = 'true')
    SELECT revision,
      (SELECT count(*) FROM jsonb_array_elements(lists) AS l
        WHERE l->>'packId' IS NULL OR l->>'packId' NOT IN (SELECT id FROM followed)) AS own,
      (SELECT count(*) FROM followed) AS followed
    FROM typle_workspaces WHERE owner_id = ?
  SQL

  # The saved row of owner: lists, packs, devices, interests, revision and its split records as JSON text, or nil.
  def self.saved(owner) = where(owner_id: owner).pick(:lists, :packs, :devices, :interests, :revision, RECORDS)

  # The revision of owner's row and its numbers of own lists and followed packs, or nil.
  def self.counts(owner)
    connection.select_rows(sanitize_sql([COUNTS, owner, owner])).first&.map(&:to_i)
  end

  # Takes the lock that keeps two first saves of owner apart until the transaction ends.
  def self.lock_first_save(owner)
    connection.select_value(sanitize_sql(["SELECT pg_advisory_xact_lock(hashtext('typle_workspaces:' || ?))", owner]))
  end
end
