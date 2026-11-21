module Typle
  # src/server/report.ts: a failure answered with a 503, in the log.
  module Report
    module_function

    def failure(request, error, operation)
      Rails.logger.error("#{request.method} #{request.path}: #{operation}: #{error.class}: #{error.message}")
    end
  end
end
