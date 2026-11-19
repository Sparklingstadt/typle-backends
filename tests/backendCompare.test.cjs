const assert = require('node:assert/strict')
const test = require('node:test')
const { backendRewrites, nextBackendHeaders, kotlinOrigin, defaultBackend, COMPARED_PATHS } = require('../scripts/backendCompare.cjs')

// The comparison of the backends (scripts/backendCompare.cjs): only a deployment built with TYPLE_BACKEND_COMPARE=1
// reads x-typle-backend, so the header changes nothing in production. TYPLE_BACKEND=go moves the compared paths to Go
// for every request, in production too.

test('without TYPLE_BACKEND_COMPARE=1 nothing is forwarded or marked', () => {
  for (const env of [{}, { TYPLE_BACKEND_COMPARE: '0' }, { TYPLE_BACKEND_COMPARE: 'true' }, { TYPLE_KOTLIN_URL: 'https://k.example' }, { TYPLE_BACKEND: 'next' }]) {
    assert.deepEqual(backendRewrites(env), [])
    assert.deepEqual(nextBackendHeaders(env), [])
  }
})

test('the comparison sends x-typle-backend: go to the Go function', () => {
  const rules = backendRewrites({ TYPLE_BACKEND_COMPARE: '1' })
  assert.deepEqual(rules, [
    { source: '/api/health', has: [{ type: 'header', key: 'x-typle-backend', value: 'go' }], destination: '/api/go?path=health' },
    { source: '/api/workspace', has: [{ type: 'header', key: 'x-typle-backend', value: 'go' }], destination: '/api/go?path=workspace' },
    { source: '/api/workspace/records', has: [{ type: 'header', key: 'x-typle-backend', value: 'go' }], destination: '/api/go?path=workspace/records' },
  ])
})

test('with TYPLE_KOTLIN_URL x-typle-backend: kotlin goes to the same path on the Kotlin service', () => {
  const rules = backendRewrites({ TYPLE_BACKEND_COMPARE: '1', TYPLE_KOTLIN_URL: 'https://typle-kotlin-abc.a.run.app/' })
  const kotlin = rules.filter(rule => rule.has[0].value === 'kotlin')
  assert.deepEqual(kotlin.map(rule => rule.destination), COMPARED_PATHS.map(source => `https://typle-kotlin-abc.a.run.app${source}`))
  assert.equal(rules.length, COMPARED_PATHS.length * 2)
})

test('TYPLE_KOTLIN_URL must be an https origin', () => {
  assert.equal(kotlinOrigin({}), null)
  assert.equal(kotlinOrigin({ TYPLE_KOTLIN_URL: '' }), null)
  for (const value of ['http://k.example', 'https://k.example/api', 'https://k.example/?x=1', 'k.example', 'https://u:p@k.example']) {
    assert.throws(() => kotlinOrigin({ TYPLE_KOTLIN_URL: value }), /TYPLE_KOTLIN_URL/, value)
  }
})

test('the Next.js answers are marked only for the requests that go to Next.js', () => {
  const rules = nextBackendHeaders({ TYPLE_BACKEND_COMPARE: '1' })
  assert.equal(rules.length, COMPARED_PATHS.length * 2)
  for (const rule of rules) assert.deepEqual(rule.headers, [{ key: 'x-typle-backend', value: 'next' }])
  assert.ok(rules.every(rule => (rule.missing?.[0].key === 'x-typle-backend') !== (rule.has?.[0].value === 'next')))
})

test('TYPLE_BACKEND=go sends every request for the compared paths to the Go function, without the comparison too', () => {
  assert.deepEqual(backendRewrites({ TYPLE_BACKEND: 'go' }), [
    { source: '/api/health', destination: '/api/go?path=health' },
    { source: '/api/workspace', destination: '/api/go?path=workspace' },
    { source: '/api/workspace/records', destination: '/api/go?path=workspace/records' },
  ])
  // Outside the comparison TYPLE_KOTLIN_URL is not read, and nothing is marked as Next.js.
  assert.equal(backendRewrites({ TYPLE_BACKEND: 'go', TYPLE_KOTLIN_URL: 'not a URL' }).length, COMPARED_PATHS.length)
  assert.deepEqual(nextBackendHeaders({ TYPLE_BACKEND: 'go' }), [])
})

test('in the comparison with TYPLE_BACKEND=go, no header means Go and next still means Next.js', () => {
  const env = { TYPLE_BACKEND_COMPARE: '1', TYPLE_BACKEND: 'go' }
  const fallbacks = backendRewrites(env).filter(rule => rule.missing)
  assert.deepEqual(fallbacks.map(rule => rule.destination), COMPARED_PATHS.map(source => `/api/go?path=${source.slice(5)}`))
  assert.ok(fallbacks.every(rule => rule.missing[0].key === 'x-typle-backend' && !rule.has))
  const marked = nextBackendHeaders(env)
  assert.equal(marked.length, COMPARED_PATHS.length)
  assert.ok(marked.every(rule => rule.has[0].value === 'next' && !rule.missing))
})

test('TYPLE_BACKEND must be next or go', () => {
  assert.equal(defaultBackend({}), 'next')
  assert.equal(defaultBackend({ TYPLE_BACKEND: '' }), 'next')
  assert.equal(defaultBackend({ TYPLE_BACKEND: 'next' }), 'next')
  assert.equal(defaultBackend({ TYPLE_BACKEND: 'go' }), 'go')
  for (const value of ['Go', 'kotlin', '1', ' go']) assert.throws(() => defaultBackend({ TYPLE_BACKEND: value }), /TYPLE_BACKEND/, value)
})
