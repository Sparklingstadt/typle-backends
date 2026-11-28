# A user of Better Auth (typle_auth_users, migration 0042 of typle-r): its id is the account ID, the owner_id of the
# account's data. The table belongs to typle-r.
class AuthUser < ApplicationRecord
  self.table_name = "typle_auth_users"
end
