# What the Next.js handlers share (src/server/response.ts, request.ts, account.ts and the checks of workspace.ts): the
# JSON answers, the checks of a write, the login and the body of a save.
class ApplicationController < ActionController::API
  MAX_BODY_BYTES = 1 << 20
  BOM = "\xEF\xBB\xBF".b.freeze

  wrap_parameters false

  # A failure nothing else answered: a 503 as the handlers answer one, in the log.
  rescue_from StandardError do |error|
    Typle::Report.failure(request, error, "#{controller_name}.#{action_name}")
    refuse(503, request.get? || request.head? ? "databaseUnavailable" : "workspaceSaveFailed")
  end

  private

  # JSON.stringify plus a newline, never cached, varying by Cookie.
  def reply(status, value)
    response.headers["Cache-Control"] = "private, no-store"
    response.headers["Vary"] = "Cookie"
    render body: Typle::JsonValue.generate(value) + "\n", status: status, content_type: "application/json; charset=utf-8"
  end

  def refuse(status, key)
    reply(status, { "error" => key })
  end

  # The login settings of AUTH_SECRET and AUTH_URL, derived once for each pair.
  def login_settings
    @login_settings ||= begin
      key = [ENV["AUTH_SECRET"], ENV["AUTH_URL"]]
      (self.class.settings_cache[key] ||= [Typle::LoginSettings.read]).first
    end
  end

  def self.settings_cache = @settings_cache ||= Concurrent::Map.new

  def require_login_settings
    refuse(503, "authUnavailable") if login_settings.nil?
  end

  # A write comes from the app's own pages (its Origin) and declares a JSON body.
  def require_write_request
    content_type = Typle::Js.trim(Typle::Js.header(request.get_header(Typle::BackendGate::CONTENT_TYPE).to_s).split(";").first.to_s)
    origin = Typle::Js.header(request.headers["Origin"])
    refuse(403, "forbiddenRequest") if origin != login_settings.origin || content_type != "application/json"
  end

  def require_database
    refuse(503, "databaseUnavailable") unless Typle::DatabaseUrl.configured?
  end

  # signedIn of src/server/account.ts: the owner of the login cookie, unless the owner signed out of every device since.
  def require_owner
    settings = login_settings
    session = Typle::Sessions.cookie_value(request.headers["Cookie"], settings.cookie_name)&.then { |value| Typle::Sessions.open(settings.session_key, value) }
    return refuse(401, "loginRequired") if session.nil?

    revoked_at = begin
      SessionRevocation.revoked_at_ms(session.sub)
    rescue StandardError => e
      Typle::Report.failure(request, e, "session.check")
      return refuse(503, "databaseUnavailable")
    end
    if revoked_at && session.issued_at * 1000 <= revoked_at
      response.headers["Set-Cookie"] = "#{settings.cookie_name}=; Path=/; Max-Age=0; HttpOnly#{"; Secure" if settings.secure}; SameSite=Lax"
      return refuse(401, "loginRequired")
    end
    @owner = session.sub
  end

  # readSaveRequest: the body as an object whose accountId is the owner's, or nil once a refusal is answered.
  def save_fields(invalid)
    body = request.body.read(MAX_BODY_BYTES + 1).to_s
    return refuse(400, "emptyRequest") && nil if body.empty?
    return refuse(413, "workspaceTooLarge") && nil if body.bytesize > MAX_BODY_BYTES

    fields = begin
      Typle::JsonValue.parse(decode_body(body))
    rescue JSON::ParserError
      nil
    end
    return refuse(400, invalid) && nil unless fields.is_a?(Hash)
    return refuse(401, "loginAccountChanged") && nil if !fields.key?("accountId") || fields["accountId"] != @owner

    fields
  end

  # The body as new TextDecoder().decode reads it: UTF-8, a leading BOM dropped, malformed bytes as U+FFFD.
  def decode_body(bytes)
    bytes = bytes.byteslice(3..) if bytes.start_with?(BOM)
    bytes.force_encoding(Encoding::UTF_8).scrub("�")
  end
end
