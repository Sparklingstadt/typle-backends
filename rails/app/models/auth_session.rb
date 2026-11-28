# A session of Better Auth (typle_auth_sessions, migration 0042 of typle-r): the login cookie names one by its token, and
# signing out of every device deletes the user's rows. The table belongs to typle-r.
class AuthSession < ApplicationRecord
  self.table_name = "typle_auth_sessions"

  belongs_to :user, class_name: "AuthUser"

  # The user of the session whose token is token, as Better Auth's findSession and getSession read it: the session's row
  # and its user's, in one statement, and only while the session has not expired at now (expires_at not before now).
  # nil for a token of no session and for an expired one.
  def self.user_of(token, now)
    sub, name, image = joins(:user).where(token: token, expires_at: now..)
      .pick("typle_auth_users.id", "typle_auth_users.name", "typle_auth_users.image")
    sub && Typle::Session.new(sub, name, image.to_s)
  end
end
