// The comparison of the backends (docs/superpowers/specs/2026-11-19-backend-compare-design.md): a deployment built with
// TYPLE_BACKEND_COMPARE=1 sends a request for one of these paths to the Go function, or to the Kotlin or Rails service on
// Cloud Run (docs/superpowers/specs/2026-11-21-rails-backend-design.md), when its x-typle-backend header names that backend, and to the Next.js Route Handlers otherwise. Without the
// variable there are no such rewrites, so the header changes nothing. Apart from the comparison, TYPLE_BACKEND=go sends
// every request for these paths to the Go function, in any environment, production included (docs/backend-switch.md).
// next.config.mjs and scripts/bench-backends.cjs share this list.

/** The header that picks the backend of a request, and that each backend answers with its own name. */
const BACKEND_HEADER = 'x-typle-backend'

/** The paths the comparison covers, as next.config.mjs sources. */
const COMPARED_PATHS = ['/api/health', '/api/workspace', '/api/workspace/records']

/**
 * Where the Vercel Go function (api/go.go) is served. Vercel maps the file to this one path and no paths below it, so
 * the compared path goes in the path query, which the function routes on (as it does on the original path).
 */
const GO_FUNCTION_PATH = '/api/go'

/** Whether env turns the comparison on. */
function compareEnabled(env = process.env) {
  return env.TYPLE_BACKEND_COMPARE === '1'
}

/**
 * The backend that answers the compared paths when no x-typle-backend header picks one: TYPLE_BACKEND, 'next' when it
 * is not set. Throws on any other value, so a typo fails the build instead of leaving production on another backend.
 */
function defaultBackend(env = process.env) {
  const value = env.TYPLE_BACKEND
  if (value === undefined || value === '' || value === 'next') return 'next'
  if (value === 'go') return 'go'
  throw new Error(`TYPLE_BACKEND (${value}) は next か go にしてください。`)
}

/** The rewrite of a compared path to the Go function. */
function toGo(source) {
  return `${GO_FUNCTION_PATH}?path=${source.slice('/api/'.length)}`
}

/**
 * The origin of a service in the environment variable name, without a trailing slash; null when it is not set. Throws
 * when it is not an https URL with only an origin, so a typo fails the build instead of sending requests somewhere else.
 */
function serviceOrigin(name, env) {
  const value = env[name]
  if (value === undefined || value === '') return null
  let url
  try {
    url = new URL(value)
  } catch {
    throw new Error(`${name} (${value}) は URL にしてください。`)
  }
  if (url.protocol !== 'https:' || url.pathname !== '/' || url.search !== '' || url.hash !== '' || url.username !== '' || url.password !== '') {
    throw new Error(`${name} (${value}) は https:// のオリジンだけにしてください。`)
  }
  return url.origin
}

/** The Kotlin service's origin from TYPLE_KOTLIN_URL (serviceOrigin). */
function kotlinOrigin(env = process.env) {
  return serviceOrigin('TYPLE_KOTLIN_URL', env)
}

/** The Rails service's origin from TYPLE_RAILS_URL (serviceOrigin). */
function railsOrigin(env = process.env) {
  return serviceOrigin('TYPLE_RAILS_URL', env)
}

/**
 * The beforeFiles rewrites of next.config.mjs. With TYPLE_BACKEND=go, the compared paths go to the Go function: every
 * request outside the comparison, and those without x-typle-backend in it (next then still picks Next.js). In the
 * comparison, x-typle-backend picks Go, Kotlin or Rails. None when neither is on.
 */
function backendRewrites(env = process.env) {
  const compare = compareEnabled(env)
  const fallback = defaultBackend(env)
  const services = compare ? [['kotlin', kotlinOrigin(env)], ['rails', railsOrigin(env)]] : []
  return COMPARED_PATHS.flatMap((source) => {
    const rules = []
    if (compare) {
      rules.push({ source, has: [{ type: 'header', key: BACKEND_HEADER, value: 'go' }], destination: toGo(source) })
      for (const [name, origin] of services) {
        if (origin !== null) rules.push({ source, has: [{ type: 'header', key: BACKEND_HEADER, value: name }], destination: `${origin}${source}` })
      }
    }
    if (fallback === 'go') {
      rules.push(compare
        ? { source, missing: [{ type: 'header', key: BACKEND_HEADER }], destination: toGo(source) }
        : { source, destination: toGo(source) })
    }
    return rules
  })
}

/**
 * The headers rules of next.config.mjs that mark the Next.js answers with x-typle-backend: next, for the requests that go
 * to Next.js (no header, or next); none unless the comparison is on.
 */
function nextBackendHeaders(env = process.env) {
  if (!compareEnabled(env)) return []
  const headers = [{ key: BACKEND_HEADER, value: 'next' }]
  // Without the header a request goes to Next.js only while TYPLE_BACKEND is next; with go it goes to the Go function,
  // which marks its own answers.
  return COMPARED_PATHS.flatMap((source) => [
    ...(defaultBackend(env) === 'next' ? [{ source, missing: [{ type: 'header', key: BACKEND_HEADER }], headers }] : []),
    { source, has: [{ type: 'header', key: BACKEND_HEADER, value: 'next' }], headers },
  ])
}

module.exports = { BACKEND_HEADER, COMPARED_PATHS, compareEnabled, defaultBackend, kotlinOrigin, railsOrigin, backendRewrites, nextBackendHeaders }
