# /api/workspace: GET reads the owner's workspace, PUT saves a change of it by revision.
class WorkspacesController < ApplicationController
  before_action :require_login_settings
  before_action :require_write_request, only: :update
  before_action :require_database
  before_action :require_owner

  def show
    saved = begin
      Typle::WorkspaceStore.read(@owner)
    rescue StandardError => e
      Typle::Report.failure(request, e, "workspace.read")
      return refuse(503, "databaseUnavailable")
    end
    reply(200, Typle::WorkspaceStore.answer(saved, @owner))
  end

  def update
    fields = save_fields("invalidWorkspace") or return
    status, revision, change = Typle::WorkspaceChanges.read(fields)
    return refuse(400, revision) if status == :refused

    reply(200, Typle::WorkspaceStore.answer(Typle::WorkspaceStore.write(@owner, revision, change), @owner))
  rescue Typle::Conflict
    refuse(409, "conflict")
  rescue Typle::AppError => e
    raise unless %w[listLimit followLimit invalidWorkspace].include?(e.key)

    refuse(400, e.key)
  rescue StandardError => e
    Typle::Report.failure(request, e, "workspace.write")
    refuse(503, "workspaceSaveFailed")
  end
end
