module Typle
  # A signed-in user: sub is the account ID (typle_auth_users.id, the owner_id of the account's data).
  Session = Struct.new(:sub, :name, :image)
end
