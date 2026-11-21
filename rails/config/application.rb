require_relative "boot"

require "rails"
require "active_model/railtie"
require "active_record/railtie"
require "action_controller/railtie"
require "rails/test_unit/railtie"

Bundler.require(*Rails.groups)

module TypleRails
  # The Rails backend of the comparison of the backends (docs/backend-compare.md): /api/health, /api/workspace and
  # /api/workspace/records answered as the Next.js Route Handlers answer them.
  class Application < Rails::Application
    config.load_defaults 8.1
    config.api_only = true

    # Rails' session key is never used (the login cookie is Auth.js's), so a random one does when none is given.
    config.secret_key_base = ENV["SECRET_KEY_BASE"].presence || SecureRandom.hex(64)

    # Nothing Next.js does not do either: no ETag (it hashes every body), no conditional GET, no cookie jar, no request id.
    config.middleware.delete Rack::ETag
    config.middleware.delete Rack::ConditionalGet
    config.middleware.delete ActionDispatch::RequestId
    config.middleware.delete Rack::Runtime
    # The headers of every answer are those of next.config.mjs, which Typle::BackendGate adds.
    config.action_dispatch.default_headers = {}

    # The schema belongs to Next.js (db/ of the repository); Rails neither loads nor dumps one.
    config.active_record.maintain_test_schema = false
    config.active_record.schema_format = :sql
  end
end
