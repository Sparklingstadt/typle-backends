# src/server/health.ts: 200 once the database answers within 5 seconds, 503 databaseUnavailable otherwise.
class HealthController < ApplicationController
  def show
    raise "DATABASE_URL is not set" unless Typle::DatabaseUrl.configured?

    ActiveRecord::Base.connection.select_value("SELECT 1")
    reply(200, { "status" => "ok" })
  rescue StandardError => e
    Typle::Report.failure(request, e, "health")
    refuse(503, "databaseUnavailable")
  end
end
