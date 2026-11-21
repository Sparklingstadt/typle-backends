module Typle
  # src/server/workspace.ts and records.ts: reading, saving and changing a workspace in typle_workspaces, with its
  # records in typle_records. The statements are those of the Next.js handlers, one for one.
  module WorkspaceStore
    Saved = Struct.new(:lists, :packs, :devices, :interests, :revision)

    module_function

    # The owner's saved workspace, or nil; checked as the app reads it, answered as saved.
    def read(owner)
      lists, packs, devices, interests, revision, records = Workspace.saved(owner)
      return nil if revision.nil?

      if records
        stored = {}
        JsonValue.parse(records).each { |list_id, record| (stored[list_id] ||= []) << record }
        lists = RecordRows.with_records(lists, stored)
      end
      saved = Saved.new(lists, packs, devices, interests, revision)
      # Raises on saved data the app could not read either.
      document = WorkspaceDocuments.decode(lists, packs, devices, interests.nil? ? Value::UNDEFINED : interests)
      Catalog.followed(document.library)
      saved
    end

    # The answer of GET and PUT /api/workspace; revision 0 and no lists when nothing is saved.
    def answer(saved, owner)
      {
        "lists" => saved&.lists || [], "packs" => saved&.packs, "devices" => saved&.devices || [],
        "interests" => saved&.interests || Interests::DEFAULT_INTERESTS.dup, "revision" => saved&.revision || 0,
        "accountId" => owner,
      }
    end

    def json(value) = value.nil? ? nil : JsonValue.generate(value)

    # writeWorkspace: saves change over the owner's workspace at revision (0 creates the row), else Conflict.
    def write(owner, revision, change)
      encoded = WorkspaceChanges.encode(change)
      library = encoded.library
      Workspace.transaction do
        if revision.zero?
          raise AppError, "listLimit" if library && library.own > Limits::MAX_LISTS
          raise AppError, "followLimit" if library && library.followed > Limits::MAX_FOLLOWED_PACKS

          # Two first saves of the same owner wait here for each other.
          Workspace.lock_first_save(owner)
          inserted = Workspace.insert_all(
            [{ owner_id: owner, lists: library ? library.lists : [], packs: library&.packs, devices: encoded.devices || [],
               interests: encoded.interests, records_split: true }],
            unique_by: :owner_id, returning: %w[revision], record_timestamps: false,
          )
          raise Conflict if inserted.rows.empty?

          PracticeRecord.where(owner_id: owner).delete_all
        else
          saved_revision, own, followed = Workspace.counts(owner)
          raise Conflict if saved_revision != revision
          raise AppError, "listLimit" if library && library.own > Limits::MAX_LISTS && library.own > own
          raise AppError, "followLimit" if library && library.followed > Limits::MAX_FOLLOWED_PACKS && library.followed > followed

          # Matching the revision in the UPDATE itself makes the comparison and the write one atomic operation.
          lists = json(library&.lists)
          updated = Workspace.where(owner_id: owner, revision: revision).update_all([<<~SQL.squish, lists, json(library&.packs), json(encoded.devices), json(encoded.interests), lists])
            lists = COALESCE(?::jsonb, lists), packs = COALESCE(?::jsonb, packs), devices = COALESCE(?::jsonb, devices),
            interests = COALESCE(?::jsonb, interests), records_split = records_split OR ?::jsonb IS NOT NULL,
            revision = revision + 1, updated_at = now()
          SQL
          raise Conflict if updated.zero?
        end
        sync_records(owner, library.rows) if library
        read(owner)
      end
    end

    # modifyWorkspace: changes the owner's workspace in one transaction that holds the row. The block takes the
    # document and returns [next document, whether it changed, result]; returns [result, the saved workspace].
    def modify(owner)
      Workspace.transaction do
        row = Workspace.lock.where(owner_id: owner).pick(:lists, :packs, :devices, :records_split)
        document = WorkspaceDocument.new([], [], [], [])
        if row
          lists, packs, devices, split = row
          if split
            stored = {}
            PracticeRecord.where(owner_id: owner).order(:list_id, :position).pluck(:list_id, :data).each do |list_id, data|
              (stored[list_id] ||= []) << data
            end
            lists = RecordRows.with_records(lists, stored)
          end
          decoded = WorkspaceDocuments.decode(lists, packs, devices, Value::UNDEFINED)
          followed = Catalog.followed(decoded.library)
          document = WorkspaceDocument.new(followed.packs, followed.lists, decoded.devices, decoded.interests)
        end
        before = WorkspaceChanges.encode(WorkspaceChange.new(document.lists, document.packs)).library.size
        changed_document, changed, result = yield document
        if changed
          raise "no workspace of #{owner} to change" if row.nil?

          library = WorkspaceChanges.encode(WorkspaceChange.new(changed_document.lists, changed_document.packs, changed_document.devices)).library
          raise AppError, "workspaceTooLarge" if library.size > WorkspaceChanges::MAX_LISTS_BYTES && library.size > before

          Workspace.where(owner_id: owner).update_all([<<~SQL.squish, json(library.lists), json(library.packs), json(changed_document.devices)])
            lists = ?::jsonb, packs = ?::jsonb, devices = ?::jsonb, records_split = true, revision = revision + 1, updated_at = now()
          SQL
          sync_records(owner, library.rows)
        end
        [result, read(owner)]
      end
    end

    # Makes the owner's rows of typle_records exactly rows (syncRecords): the rows of records no longer saved are deleted,
    # the others written where they changed, and the number of rows checked afterwards.
    def sync_records(owner, rows)
      PracticeRecord.delete_all_but(owner, rows.map { |row| [row.list_id, row.record_id] })
      unless rows.empty?
        PracticeRecord.write_all(rows.map do |row|
          { owner_id: owner, list_id: row.list_id, record_id: row.record_id, position: row.position,
            at: timestamp(row.at), data: JsonType::Raw.new(row.data) }
        end)
      end
      counted = PracticeRecord.where(owner_id: owner).count
      raise "records of #{owner}: #{counted} rows saved for #{rows.size} records" if counted != rows.size
    end

    # A timestamptz PostgreSQL reads, of a time in milliseconds (node-postgres writes a Date so, in UTC here).
    def timestamp(time)
      iso = JsDate.to_iso(time)
      year = JsDate.utc_year(time).to_i
      return iso if year.between?(1, 9999)

      rest = iso[iso.index("-", 1)..]
      year > 9999 ? "#{year}#{rest}" : "#{(1 - year).to_s.rjust(4, "0")}#{rest.delete_suffix("Z")}Z BC"
    end
  end
end
