# next_build_number yields to a block that returns the latest TestFlight
# build number (or raises / returns nil when the app record is brand new
# and has no build yet). It returns the next number to use: the first ever
# release is 1, and a real latest increments by one. Keeping this out of
# the Fastfile DSL lets build_number_test.rb exercise it in plain Ruby.
def next_build_number
  latest = (yield rescue nil)
  latest = 0 if latest.nil? || latest < 0
  latest + 1
end
