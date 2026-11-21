const assert = require('node:assert/strict')
const test = require('node:test')
const { parseArgs, percentile, summarize, targetsFor } = require('../scripts/bench-backends.cjs')

test('the arguments: the deployment, the Kotlin service and the counts', () => {
  const options = parseArgs(['https://p.example/x', '--kotlin', 'https://k.example/', '--rounds', '10', '--warmup', '0', '--only', 'health,records'])
  assert.deepEqual(options, { url: 'https://p.example', kotlin: 'https://k.example', rails: null, rounds: 10, warmup: 0, origin: 'https://p.example', only: ['health', 'records'] })
  assert.equal(parseArgs(['https://p.example', '--rails', 'https://r.example/x']).rails, 'https://r.example')
  assert.equal(parseArgs(['https://p.example', '--origin', 'https://typle.example/']).origin, 'https://typle.example')
})

test('wrong arguments are refused', () => {
  assert.throws(() => parseArgs([]), /URL/)
  assert.throws(() => parseArgs(['https://p.example', '--rounds', '0']), /--rounds/)
  assert.throws(() => parseArgs(['https://p.example', '--warmup', '-1']), /--warmup/)
  assert.throws(() => parseArgs(['https://p.example', '--only', 'auth']), /--only/)
  assert.throws(() => parseArgs(['https://p.example', '--kotlin']), /--kotlin/)
  assert.throws(() => parseArgs(['https://p.example', '--rails']), /--rails/)
  assert.throws(() => parseArgs(['https://p.example', '--fast']), /--fast/)
})

test('percentiles by the nearest rank', () => {
  const sorted = Array.from({ length: 100 }, (_, i) => i + 1)
  assert.equal(percentile(sorted, 50), 50)
  assert.equal(percentile(sorted, 95), 95)
  assert.equal(percentile([7], 95), 7)
  assert.ok(Number.isNaN(percentile([], 50)))
  assert.deepEqual(summarize([3, 1, 2], 1), { n: 3, p50: 2, p95: 3, max: 3, failures: 1 })
})

test('Rails is measured only with --rails, and Kotlin directly only with --kotlin', () => {
  assert.deepEqual(targetsFor(parseArgs(['https://p.example'])), ['next', 'go', 'kotlin'])
  assert.deepEqual(targetsFor(parseArgs(['https://p.example', '--kotlin', 'https://k.example'])), ['next', 'go', 'kotlin', 'kotlin-direct'])
  assert.deepEqual(targetsFor(parseArgs(['https://p.example', '--rails', 'https://r.example'])), ['next', 'go', 'kotlin', 'rails', 'rails-direct'])
})
