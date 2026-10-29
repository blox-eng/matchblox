# Plain-Ruby test for the build-number rescue, independent of the fastlane
# DSL. Run: ruby fastlane/build_number_test.rb
require_relative "build_number"

def check(desc, got, want)
  raise "FAIL: #{desc}: got #{got.inspect}, want #{want.inspect}" unless got == want
  puts "ok: #{desc} => #{got}"
end

# First ever release: App Store Connect has no build / the version is not
# there yet, so the query raises. The lane must still produce 1.
check("raises (no app/version/build yet)", next_build_number { raise "not found" }, 1)
# A zero/absent latest also means "start at 1".
check("zero latest", next_build_number { 0 }, 1)
check("negative/sentinel latest", next_build_number { -1 }, 1)
check("nil latest", next_build_number { nil }, 1)
# A real latest increments.
check("latest 7 -> 8", next_build_number { 7 }, 8)

puts "ALL PASS"
