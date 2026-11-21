ENV["RAILS_ENV"] ||= "test"
require_relative "../config/environment"
require "rails/test_help"
require "minitest/mock"

module ActiveSupport
  class TestCase
    # The DB tests check the rows other connections leave (two saves at once), so no test runs in a transaction.
    self.use_transactional_tests = false

    # Sets the environment variables for the block and puts them back afterwards.
    def with_env(values)
      saved = values.keys.to_h { |key| [key, ENV[key]] }
      values.each { |key, value| value.nil? ? ENV.delete(key) : ENV[key] = value }
      yield
    ensure
      saved.each { |key, value| value.nil? ? ENV.delete(key) : ENV[key] = value }
    end
  end
end

# The shared fixtures of the repository (tests/fixtures), read in place: Next.js, Go and Kotlin are held to the same files.
module Fixtures
  DIR = Rails.root.join("../tests/fixtures")

  module_function

  def read(path)
    Typle::JsonValue.parse(File.read(DIR.join(path)))
  end

  # The JSON files of a fixture directory, by name.
  def files(path)
    Dir[DIR.join(path, "*.json")].sort.map { |file| [File.basename(file), Typle::JsonValue.parse(File.read(file))] }
  end
end
