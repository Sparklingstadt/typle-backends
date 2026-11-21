Rails.application.configure do
  config.enable_reloading = false
  config.eager_load = ENV["CI"].present?
  config.consider_all_requests_local = false
  config.cache_store = :null_store
  config.action_dispatch.show_exceptions = :all
  config.active_support.deprecation = :stderr
  config.hosts.clear
end
