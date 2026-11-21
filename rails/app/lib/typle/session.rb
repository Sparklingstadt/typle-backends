module Typle
  # A signed-in user; times are Unix seconds.
  Session = Struct.new(:sub, :name, :image, :issued_at, :expires)
end
