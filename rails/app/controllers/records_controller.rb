# /api/workspace/records: POST adds a practice record to a list, DELETE deletes one.
class RecordsController < ApplicationController
  RECORD_FIELDS = %w[id time date mistakes accuracy wordCount deviceId device mode missed].freeze

  before_action :require_login_settings
  before_action :require_write_request
  before_action :require_database
  before_action :require_owner

  def create
    fields = save_fields("invalidRecord") or return
    list_id, input = read_add(fields)
    return refuse(400, "invalidRecord") if list_id.nil?

    result, saved = Typle::WorkspaceStore.modify(@owner) do |document|
      added = Typle::Records.add_record(document.lists, document.devices, list_id, input)
      [Typle::WorkspaceDocument.new(document.packs, added.lists, document.devices, document.interests), added.added, added]
    end
    return refusal(RuntimeError.new("workspace missing after a record was added"), "record.add") if saved.nil?

    reply(result.added ? 201 : 200, Typle::WorkspaceStore.answer(saved, @owner).merge("record" => result.record))
  rescue StandardError => e
    refusal(e, "record.add")
  end

  def destroy
    fields = save_fields("invalidRecord") or return
    list_id, record_id = read_delete(fields)
    return refuse(400, "invalidRecord") if list_id.nil?

    _, saved = Typle::WorkspaceStore.modify(@owner) do |document|
      lists = Typle::Records.delete_record(document.lists, list_id, record_id)
      [lists ? Typle::WorkspaceDocument.new(document.packs, lists, document.devices, document.interests) : document, !lists.nil?, nil]
    end
    return refusal(RuntimeError.new("workspace missing after a record was deleted"), "record.delete") if saved.nil?

    reply(200, Typle::WorkspaceStore.answer(saved, @owner))
  rescue StandardError => e
    refusal(e, "record.delete")
  end

  private

  # readRecordAdd: the list and record of a body that is exactly accountId, listId and record.
  def read_add(fields)
    list_id = fields["listId"]
    record = fields["record"]
    return nil if fields.size != 3 || !list_id.is_a?(String) || list_id.empty? || !record.is_a?(Hash)

    input = record.dup
    return nil if input.keys.any? { |key| !RECORD_FIELDS.include?(key) } || !input["id"].is_a?(String)

    %w[wordCount device deviceId].each { |name| input.delete(name) if input.key?(name) && input[name].nil? }
    [list_id, input]
  end

  # readRecordDelete: the list and record id of a body that is exactly accountId, listId and recordId.
  def read_delete(fields)
    list_id = fields["listId"]
    record_id = fields["recordId"]
    return nil if fields.size != 3 || !list_id.is_a?(String) || list_id.empty? || !record_id.is_a?(String) || record_id.empty?

    [list_id, record_id]
  end

  def refusal(error, operation)
    if error.is_a?(Typle::AppError)
      return refuse(404, "recordListDeleted") if error.key == "recordListDeleted"
      return refuse(413, "workspaceTooLarge") if error.key == "workspaceTooLarge"

      return refuse(400, error.key)
    end
    Typle::Report.failure(request, error, operation)
    refuse(503, "workspaceSaveFailed")
  end
end
