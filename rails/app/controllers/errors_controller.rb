# The answers of the paths and methods no route file serves: 405 with the methods the path serves, and 404.
class ErrorsController < ApplicationController
  def method_not_allowed
    response.headers["Allow"] = request.path_parameters[:allow]
    refuse(405, "methodNotAllowed")
  end

  def not_found
    refuse(404, "notFound")
  end
end
