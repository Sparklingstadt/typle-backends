#!/usr/bin/env node
// Times the compared paths (scripts/backendCompare.cjs) on a deployment built with TYPLE_BACKEND_COMPARE=1: each round
// sends the same request to Next.js, Go and Kotlin through the deployment (x-typle-backend), and to the Kotlin service
// directly when --kotlin is given, in turn, so that all of them meet the same database and the same hour. Prints p50,
// p95 and the slowest round trip of each, in milliseconds, and the failures.
//
// Usage:
//   TYPLE_BENCH_COOKIE='__Host-typle-session=…' node scripts/bench-backends.cjs <deployment URL> [--kotlin <URL>]
//     [--rounds 50] [--warmup 3] [--origin <URL>] [--only health,workspace.get,workspace.put,records]
//
// TYPLE_BENCH_COOKIE is the Cookie header of a test account signed in on the production site (the preview has no OAuth
// callback of its own); only that account's workspace is written: PUT writes back what GET read, and each record added
// is deleted again. VERCEL_AUTOMATION_BYPASS_SECRET, when set, passes Vercel's Deployment Protection. --warmup 0 times
// cold starts too. Writes are sent with the Origin of --origin (by default the deployment's), which must be the
// AUTH_URL the backends were given.

const { BACKEND_HEADER } = require('./backendCompare.cjs')

const TARGETS = ['next', 'go', 'kotlin', 'kotlin-direct']
const OPERATIONS = ['health', 'workspace.get', 'workspace.put', 'records']

function parseArgs(argv) {
  const options = { url: null, kotlin: null, rounds: 50, warmup: 3, origin: null, only: OPERATIONS }
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i]
    const value = () => {
      if (i + 1 >= argv.length) throw new Error(`${arg} に値がありません。`)
      return argv[++i]
    }
    if (arg === '--kotlin') options.kotlin = value()
    else if (arg === '--rounds') options.rounds = count(value(), arg, 1)
    else if (arg === '--warmup') options.warmup = count(value(), arg, 0)
    else if (arg === '--origin') options.origin = value()
    else if (arg === '--only') options.only = value().split(',')
    else if (arg.startsWith('--')) throw new Error(`知らないオプションです: ${arg}`)
    else if (options.url === null) options.url = arg
    else throw new Error(`引数が多すぎます: ${arg}`)
  }
  if (options.url === null) throw new Error('デプロイの URL を指定してください。')
  for (const name of options.only) if (!OPERATIONS.includes(name)) throw new Error(`--only は ${OPERATIONS.join('・')} から選んでください: ${name}`)
  options.url = new URL(options.url).origin
  if (options.kotlin !== null) options.kotlin = new URL(options.kotlin).origin
  options.origin = options.origin === null ? options.url : new URL(options.origin).origin
  return options
}

function count(text, name, min) {
  const value = Number(text)
  if (!Number.isSafeInteger(value) || value < min) throw new Error(`${name} は ${min} 以上の整数にしてください: ${text}`)
  return value
}

/** The p-th percentile (0–100) of sorted, by the nearest rank. */
function percentile(sorted, p) {
  if (sorted.length === 0) return NaN
  return sorted[Math.min(sorted.length - 1, Math.max(0, Math.ceil(p / 100 * sorted.length) - 1))]
}

/** The summary row of a target's timings and failures. */
function summarize(times, failures) {
  const sorted = [...times].sort((a, b) => a - b)
  return { n: sorted.length, p50: percentile(sorted, 50), p95: percentile(sorted, 95), max: sorted.at(-1) ?? NaN, failures }
}

function makeClient(options, env) {
  const cookie = env.TYPLE_BENCH_COOKIE
  if (!cookie) throw new Error('TYPLE_BENCH_COOKIE に、本番でログインしたテスト用アカウントの Cookie を入れてください。')
  const bypass = env.VERCEL_AUTOMATION_BYPASS_SECRET

  /** One request to target; the time is from sending to the end of the body. */
  return async function send(target, method, path, body) {
    const direct = target === 'kotlin-direct'
    if (direct && options.kotlin === null) throw new Error('--kotlin がありません。')
    const headers = { Cookie: cookie, Accept: 'application/json' }
    if (!direct) {
      headers[BACKEND_HEADER] = target
      if (bypass) headers['x-vercel-protection-bypass'] = bypass
    }
    if (body !== undefined) {
      headers['Content-Type'] = 'application/json'
      headers.Origin = options.origin
    }
    const started = performance.now()
    const response = await fetch(`${direct ? options.kotlin : options.url}${path}`, {
      method, headers, body: body === undefined ? undefined : JSON.stringify(body), redirect: 'manual',
    })
    const text = await response.text()
    const ms = performance.now() - started
    const answeredBy = response.headers.get(BACKEND_HEADER)
    const expected = direct ? 'kotlin' : target
    let json = null
    try {
      json = JSON.parse(text)
    } catch { /* not JSON: judged by the status alone */ }
    return { ms, status: response.status, json, answeredBy, rightBackend: answeredBy === expected }
  }
}

/** The steps of one operation for one target: an untimed preparation, then the timed request. */
function operationFor(name, send) {
  switch (name) {
    case 'health':
      return async target => send(target, 'GET', '/api/health')
    case 'workspace.get':
      return async target => send(target, 'GET', '/api/workspace')
    case 'workspace.put':
      return async (target) => {
        const read = await send(target, 'GET', '/api/workspace')
        if (read.status !== 200 || !read.json) return { ...read, ms: NaN, note: `GET ${read.status}` }
        const { accountId, revision, lists, packs, devices, interests } = read.json
        const body = { accountId, revision, lists, devices, interests }
        if (packs !== null) body.packs = packs
        return send(target, 'PUT', '/api/workspace', body)
      }
    case 'records':
      return async (target) => {
        const read = await send(target, 'GET', '/api/workspace')
        if (read.status !== 200 || !read.json) return { ...read, ms: NaN, note: `GET ${read.status}` }
        const list = read.json.lists[0]
        if (!list) return { ...read, ms: NaN, note: 'テスト用アカウントにリストがありません' }
        const recordId = `bench-${Date.now()}-${Math.random().toString(36).slice(2)}`
        const record = { id: recordId, time: 10, date: new Date().toISOString(), mistakes: 0, accuracy: 100, wordCount: 1 }
        const added = await send(target, 'POST', '/api/workspace/records', { accountId: read.json.accountId, listId: list.id, record })
        const deleted = await send(target, 'DELETE', '/api/workspace/records', { accountId: read.json.accountId, listId: list.id, recordId })
        // The time is the add and the delete together; either failing fails the round.
        const ok = added.status === 201 && deleted.status === 200
        return { ...added, ms: added.ms + deleted.ms, status: ok ? 200 : (added.status === 201 ? deleted.status : added.status),
          rightBackend: added.rightBackend && deleted.rightBackend }
      }
    default:
      throw new Error(name)
  }
}

const OK = { health: 200, 'workspace.get': 200, 'workspace.put': 200, records: 200 }

async function main(argv = process.argv.slice(2), env = process.env) {
  const options = parseArgs(argv)
  const send = makeClient(options, env)
  const targets = TARGETS.filter(target => target !== 'kotlin-direct' || options.kotlin !== null)

  // Signed in as the account on every backend? A 401 here usually means the preview's AUTH_SECRET is not production's.
  for (const target of targets) {
    const check = await send(target, 'GET', '/api/workspace')
    if (!check.rightBackend) throw new Error(`${target}: ${BACKEND_HEADER} が ${check.answeredBy ?? 'なし'} でした。比較モード（TYPLE_BACKEND_COMPARE=1）のデプロイか、TYPLE_KOTLIN_URL を確かめてください。`)
    if (check.status === 401) throw new Error(`${target}: 401 でした。Cookie が古いか、プレビューの AUTH_SECRET が本番と違います。`)
    if (check.status !== 200) throw new Error(`${target}: GET /api/workspace が ${check.status} でした。`)
  }

  const rows = []
  for (const name of options.only) {
    const run = operationFor(name, send)
    for (let i = 0; i < options.warmup; i++) for (const target of targets) await run(target)
    const times = Object.fromEntries(targets.map(target => [target, []]))
    const failures = Object.fromEntries(targets.map(target => [target, 0]))
    const notes = new Set()
    for (let round = 0; round < options.rounds; round++) {
      for (const target of targets) {
        const result = await run(target)
        if (result.status === OK[name] && result.rightBackend && Number.isFinite(result.ms)) times[target].push(result.ms)
        else {
          failures[target]++
          notes.add(`${target}: ${result.note ?? `${result.status}${result.json?.error ? ` ${result.json.error}` : ''}${result.rightBackend ? '' : ` (${BACKEND_HEADER}: ${result.answeredBy})`}`}`)
        }
      }
    }
    for (const target of targets) rows.push({ operation: name, target, ...summarize(times[target], failures[target]) })
    for (const note of notes) console.error(`  ${name} の失敗: ${note}`)
  }
  printTable(rows, options)
}

function printTable(rows, options) {
  const fmt = value => Number.isFinite(value) ? value.toFixed(1) : '-'
  console.log(`# ${options.url}（${options.rounds} 回、温め ${options.warmup} 回）`)
  console.log('| 操作 | バックエンド | 回数 | p50 (ms) | p95 (ms) | 最大 (ms) | 失敗 |')
  console.log('|---|---|--:|--:|--:|--:|--:|')
  for (const row of rows) console.log(`| ${row.operation} | ${row.target} | ${row.n} | ${fmt(row.p50)} | ${fmt(row.p95)} | ${fmt(row.max)} | ${row.failures} |`)
}

if (require.main === module) {
  main().catch((error) => {
    console.error(error.message)
    process.exitCode = 1
  })
}

module.exports = { parseArgs, percentile, summarize }
