Rails.application.routes.draw do
  # Every answer is JSON; a format in the params also keeps Rails from adding Vary: Accept, X-Requested-With.
  scope "/api", defaults: { format: :json } do
    # GET routes answer HEAD too (Rails runs them and drops the body).
    get "health", to: "health#show"
    match "health", to: "errors#method_not_allowed", via: :all, defaults: { allow: "GET, HEAD" }

    get "workspace", to: "workspaces#show"
    put "workspace", to: "workspaces#update"
    match "workspace", to: "errors#method_not_allowed", via: :all, defaults: { allow: "GET, HEAD, PUT" }

    post "workspace/records", to: "records#create"
    delete "workspace/records", to: "records#destroy"
    match "workspace/records", to: "errors#method_not_allowed", via: :all, defaults: { allow: "POST, DELETE" }
  end

  root to: "errors#not_found", via: :all, defaults: { format: :json }
  match "*path", to: "errors#not_found", via: :all, format: false, defaults: { format: :json }
end
