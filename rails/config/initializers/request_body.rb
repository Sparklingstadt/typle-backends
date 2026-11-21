# The handlers read the body themselves, as the Next.js handlers do (src/server/request.ts), so Rails never parses a
# JSON body into params on its way.
ActionDispatch::Request.parameter_parsers = {}
