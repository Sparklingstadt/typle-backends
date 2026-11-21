# When an owner last signed out of every device (typle_session_revocations): the logins from before it no longer count.
class SessionRevocation < ApplicationRecord
  self.table_name = "typle_session_revocations"
  self.primary_key = "owner_id"

  # The time in milliseconds, or nil.
  def self.revoked_at_ms(owner)
    time = where(owner_id: owner).pick(:revoked_at)
    time && (time.to_r * 1000).floor
  end
end
