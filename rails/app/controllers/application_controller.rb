# What the Next.js handlers share (src/server/response.ts, request.ts, account.ts and the checks of workspace.ts): the
# JSON answers, the checks of a write, the login and the body of a save.
class ApplicationController < ActionController::API
  MAX_BODY_BYTES = 1 << 20
  BOM = "\xEF\xBB\xBF".b.freeze
  # The shared demo account's ID (src/server/demoLogin.ts).
  DEMO_ACCOUNT_ID = "demo:shared"

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

  # The login settings of AUTH_URL, BETTER_AUTH_SECRET and DEMO_LOGIN, read once for each set of values.
  def login_settings
    @login_settings ||= begin
      key = [ENV["AUTH_URL"], ENV["BETTER_AUTH_SECRET"], ENV["DEMO_LOGIN"]]
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

  # signedIn of src/server/account.ts: the user of the Better Auth session the login cookie names. loginRequired without
  # a session cookie, with one whose signature does not check out, or with one of no session or an expired one;
  # databaseUnavailable when the session cannot be read (also without BETTER_AUTH_SECRET, as Better Auth then fails to
  # start), as the data cannot be either. No cookie is removed: typle-r reads the session through auth.api.getSession,
  # whose cookies do not reach the answer. The session row is always read (typle-r's cookie cache may answer for up to 5
  # minutes without it), so a session deleted by signing out of every device is refused at once. The shared demo
  # account's sessions count only while its login is on (DEMO_LOGIN, src/server/demoLogin.ts).
  def require_owner
    settings = login_settings
    if settings.secret.empty?
      Typle::Report.failure(request, RuntimeError.new("BETTER_AUTH_SECRET is not set"), "session.check")
      return refuse(503, "databaseUnavailable")
    end
    token = Typle::Sessions.cookie_value(request.headers["Cookie"], settings.cookie_name)&.then { |value| Typle::Sessions.token(value, settings.secret) }
    return refuse(401, "loginRequired") if token.nil?

    user = begin
      AuthSession.user_of(token, Time.now)
    rescue StandardError => e
      Typle::Report.failure(request, e, "session.check")
      return refuse(503, "databaseUnavailable")
    end
    return refuse(401, "loginRequired") if user.nil? || (user.sub == DEMO_ACCOUNT_ID && !settings.demo_login)

    @owner = user.sub
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
