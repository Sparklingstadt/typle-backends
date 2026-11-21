require Rails.root.join("app/middleware/typle/backend_gate")

Rails.application.config.middleware.insert_before 0, Typle::BackendGate
