Rails.application.configure do
  config.enable_reloading = false
  config.eager_load = true
  config.consider_all_requests_local = false
  # Cloud Run and Vercel terminate TLS; the app answers plain HTTP behind them.
  config.assume_ssl = true
  config.force_ssl = false
  config.hosts.clear
  # Requests are not logged (Next.js does not log them either); failures are (Typle::Report).
  config.logger = ActiveSupport::Logger.new(STDOUT)
  config.log_level = ENV.fetch("RAILS_LOG_LEVEL", "warn")
  config.active_support.report_deprecations = false
  config.active_record.dump_schema_after_migration = false
end
