# Rails バックエンド（速度比較）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `rails/` に、`/api/health`・`/api/workspace`・`/api/workspace/records` を Next.js と同じ答えで返す Rails の API を作り、`x-typle-backend: rails` で速度比較に加える。

**Architecture:** Rails 8.1 の API アプリ。ゲートとヘッダーは Rack のミドルウェア、3つのパスは `config/routes.rb` とコントローラー、表は ActiveRecord のモデル。`src/lib` の検査とパックの処理は `app/lib/typle/` のふつうの Ruby に移す（Kotlin の `kotlin/src/main/kotlin/typle/lib` が直訳の手本）。Next.js 側は `scripts/backendCompare.cjs` と `scripts/bench-backends.cjs` に Rails を足す。

**Tech Stack:** Ruby 3.4.1（YJIT）、Rails 8.1、Puma、pg、Minitest。Node.js 24（Next.js 側）。

**Spec:** `docs/superpowers/specs/2026-11-21-rails-backend-design.md`（手本のコード: `kotlin/`、Next.js の `src/server/workspace.ts`・`records.ts`・`account.ts`・`session.ts`）

## Global Constraints

- Rails は `TYPLE_BACKEND_COMPARE=1` がなければ、すべてに `404 {"error":"notFound"}\n`。どの答え（404・405・500 相当も）にも `x-typle-backend: rails` と、`X-Frame-Options: DENY`・`X-Content-Type-Options: nosniff`・`Referrer-Policy: strict-origin-when-cross-origin`・`Permissions-Policy: camera=(), microphone=(), geolocation=()`・`Content-Security-Policy: frame-ancestors 'none'; base-uri 'self'; object-src 'none'` を付ける。
- JSON の答えは `JSON.generate(value) + "\n"`、`Content-Type: application/json; charset=utf-8`、`Cache-Control: private, no-store`、`Vary: Cookie`。エラーは `{"error":"<key>"}`。
- 文字列の長さの上限（`MAX_*`）は JS の `.length`（UTF-16 の単位。絵文字は 2）で数える。trim は JS の空白（Kotlin の `isJsSpace`）。小文字化は `downcase`（Kotlin の `Locale.ROOT` 相当）。
- 読んだ JSON の数は、整数の値の Float（`|x| <= 2**53`）を Integer に直してから使う（JS が `1` と書く値を `1.0` と書かないため）。
- Cookie だけでログインを決める。`Authorization` ヘッダーは読まず、`fromApp` の扱いもしない（書き込みは `Origin` が `AUTH_URL` のオリジンと同じときだけ）。
- DB に流す文の数と種類は Kotlin の `Workspace.kt`・`RecordStore.kt`・`Account.kt` と同じ。ActiveRecord で書けるところは ActiveRecord で書く。
- 接続プール 5、Puma 1 プロセス 5 スレッド、`idle_timeout` 30 秒、`connect_timeout` 5 秒、`statement_timeout` 5000ms。`sslmode=require` は `verify-full` にする。
- `typle_save_metrics` は書かない。Rails のマイグレーションは作らない。
- 文書・コミットは日本語、コードのコメントと識別子は英語。

## Review Focus

1. 孤立サロゲート（`"\ud800"`）や U+0000 を含む本文 → `400 invalidWorkspace`。Ruby のエンコーディングの例外で 500 にしない（Task 3 のテスト）。
2. `"time":1500.0` のような整数の Float → DB にも答えにも `1500` と入る（Task 2・5 のテスト）。
3. 同じキーが2回ある JSON → あとの値が勝つ（JS と同じ）。警告や例外にしない（Task 2 のテスト）。
4. 本文がちょうど 1 MiB と 1 MiB+1 バイト → 前者は読み、後者は `413 workspaceTooLarge`。BOM は落とし、壊れた UTF-8 は U+FFFD にする（Task 5 のテスト）。
5. 予期しない例外 → Rails の HTML の 500 ではなく、ヘッダー付きの `503 {"error":"workspaceSaveFailed"}`（読み込みは `databaseUnavailable`）。ログに出す（Task 5 のテスト）。

---

### Task 1: Rails アプリの骨組み、ゲート、ルーティング

**Files:**
- Create: `rails/`（`rails new rails --api --minimal --database=postgresql --skip-git --skip-docker --skip-ci --skip-kamal --skip-thruster --skip-rubocop --skip-brakeman --skip-solid` で作り、要らないファイルは消す）
- Create: `rails/app/middleware/typle/backend_gate.rb`
- Create: `rails/app/controllers/application_controller.rb`、`rails/app/controllers/errors_controller.rb`
- Modify: `rails/config/routes.rb`、`rails/config/application.rb`、`rails/config/environments/production.rb`、`rails/config/puma.rb`、`rails/config/database.yml`
- Test: `rails/test/integration/routes_test.rb`

**Interfaces:**
- Produces: `Typle::BackendGate`（Rack ミドルウェア。`ENV["TYPLE_BACKEND_COMPARE"] == "1"` をリクエストごとに読む）、`ApplicationController#reply(status, value)`・`#refuse(status, key)`、`ErrorsController#method_not_allowed`（ルートの `defaults: { allow: "GET, HEAD" }` を `Allow` に入れる）・`#not_found`。

- [ ] **Step 1: アプリを作る。** 上のコマンドで作り、`.ruby-version` を `3.4.1` にする。`Gemfile` は `rails`・`pg`・`puma`・`bootsnap` だけ。`config/application.rb` で `Rack::ETag`・`Rack::ConditionalGet`・`ActionDispatch::Cookies` 系を外し、`ActiveSupport.escape_html_entities_in_json = false`、`config.active_record.maintain_test_schema = false`。production は `log_level :warn`、`eager_load true`、`config.hosts.clear`。YJIT が production でオンになっていることを確かめる（`RubyVM::YJIT.enabled?`）。
- [ ] **Step 2: 失敗するテストを書く**（`routes_test.rb`、`ActionDispatch::IntegrationTest`。`ENV` を変えたら `ensure` で戻す）

```ruby
test "outside the comparison every request is 404 notFound" do
  with_env("TYPLE_BACKEND_COMPARE" => nil) do
    [[:get, "/api/health"], [:get, "/api/workspace"], [:put, "/api/workspace"], [:post, "/api/workspace/records"], [:get, "/"]].each do |method, path|
      process method, path
      assert_equal 404, response.status
      assert_equal "{\"error\":\"notFound\"}\n", response.body
      assert_equal "rails", response.headers["x-typle-backend"]
      assert_equal "DENY", response.headers["X-Frame-Options"]
    end
  end
end

test "other methods are 405 with Allow" do
  with_env("TYPLE_BACKEND_COMPARE" => "1") do
    { "/api/health" => [%i[post put patch delete options], "GET, HEAD"],
      "/api/workspace" => [%i[post patch delete options], "GET, HEAD, PUT"],
      "/api/workspace/records" => [%i[get head put patch options], "POST, DELETE"] }.each do |path, (methods, allow)|
      methods.each do |method|
        process method, path
        assert_equal 405, response.status, "#{method} #{path}"
        assert_equal allow, response.headers["Allow"]
        assert_equal "private, no-store", response.headers["Cache-Control"]
        assert_equal "rails", response.headers["x-typle-backend"]
        assert_equal "{\"error\":\"methodNotAllowed\"}\n", response.body unless method == :head
      end
    end
  end
end
```

- [ ] **Step 3: 動かして失敗を見る。** `cd rails && bin/rails test test/integration/routes_test.rb` → FAIL（ルートがない）。
- [ ] **Step 4: 実装する。** ミドルウェアはゲートの 404 を自分で返し、すべての答えにヘッダーを足す（`config.middleware.insert_before 0`）。`routes.rb` は3つのパスの正しいメソッドをコントローラーに、パスごとの `match ..., via: :all` を `errors#method_not_allowed` に、`match "*path", via: :all` と `root` を `errors#not_found` に。Task 4・5 のコントローラーはまだないので、この段階ではルートの行き先を `errors#not_found` にしておき、Task 4・5 で差し替える。
- [ ] **Step 5: テストを通す。** 同じコマンド → PASS。
- [ ] **Step 6: コミット**（`git add rails && git commit -m "Rails の比較用バックエンドの骨組みとゲートを作る"`）

### Task 2: JS と同じ文字・数・日付・JSON の扱い

**Files:**
- Create: `rails/app/lib/typle/js.rb`（`Typle::Js`）、`rails/app/lib/typle/js_date.rb`（`Typle::JsDate`）、`rails/app/lib/typle/json_value.rb`（`Typle::JsonValue`）
- Test: `rails/test/lib/js_test.rb`、`rails/test/test_helper.rb`（`Fixtures.read(path)`・`Fixtures.files(dir)` を足す。`Rails.root.join("../tests/fixtures")` を読む）

**Interfaces:**
- Produces:
  - `Typle::Js.space?(char) -> bool`、`.trim(s)`、`.trim_end(s)`、`.lower(s)`、`.length(s) -> Integer`（UTF-16 単位）、`.slice_to(s, units) -> String`（サロゲートの対を割らない）、`.utf8_bytesize(s)`、`.go_float(x) -> String`、`.safe_integer?(v)`、`.integer?(v)`、`.finite_number?(v)`
  - `Typle::JsDate.parse(text) -> Float`（ミリ秒、だめなら `Float::NAN`）、`.to_iso(ms) -> String`、`.utc_year(ms) -> Float`
  - `Typle::JsonValue.parse(text) -> Object`（整数の Float を Integer に、同じキーはあとが勝つ、構文エラーは `JSON::ParserError`）、`.generate(value) -> String`、`.normalize(value)`
- 手本: `kotlin/src/main/kotlin/typle/lib/JsText.kt`、`Json.kt` の `goFloat`・`shortestDigits`。

- [ ] **Step 1: 失敗するテストを書く**

```ruby
test "dates are read as Date.parse reads them" do
  assert_equal "2026-10-02T10:00:00.000Z", Typle::JsDate.to_iso(Typle::JsDate.parse("2026-10-02T19:00:00+09:00"))
  assert_equal "2026-10-01T00:00:00.000Z", Typle::JsDate.to_iso(Typle::JsDate.parse("2026-10-01"))
  assert_equal "2026-03-03T00:00:00.000Z", Typle::JsDate.to_iso(Typle::JsDate.parse("2026-02-31"))
  assert_equal "2026-10-02T10:00:00.123Z", Typle::JsDate.to_iso(Typle::JsDate.parse("2026-10-02T10:00:00.1239Z"))
  assert_equal "2026-09-07T10:00:00.000Z", Typle::JsDate.to_iso(Typle::JsDate.parse("Mon, 07 Sep 2026 10:00:00 GMT"))
  assert_equal "2026-09-07T01:00:00.000Z", Typle::JsDate.to_iso(Typle::JsDate.parse("Mon Sep 07 2026 10:00:00 GMT+0900 (JST)"))
  ["not a date", "2026-13-01", "2026-10-01T25:00", ""].each { |bad| assert Typle::JsDate.parse(bad).nan?, bad }
end

test "numbers are written as Go writes them with %g (record-ids.json floats)" do
  Fixtures.read("record-ids.json")["floats"].each { |item| assert_equal item["go"], Typle::Js.go_float(item["value"].to_f), item["value"].to_s }
end

test "JSON keeps JS's numbers and duplicate keys" do
  assert_equal({ "a" => 2 }, Typle::JsonValue.parse('{"a":1,"a":2}'))
  assert_equal '{"t":1500,"a":2.5}', Typle::JsonValue.generate(Typle::JsonValue.parse('{"t":1500.0,"a":2.5}'))
  assert_kind_of Integer, Typle::JsonValue.parse("95.0")
end

test "lengths and slices count UTF-16 units" do
  assert_equal 2, Typle::Js.length("😀")
  assert_equal "a", Typle::Js.slice_to("a😀", 2)
  assert_equal "x", Typle::Js.trim("　x﻿")
end
```

- [ ] **Step 2: 動かして失敗を見る。** `bin/rails test test/lib/js_test.rb` → FAIL（`Typle::Js` がない）。
- [ ] **Step 3: 実装する。** `JsDate` は Kotlin の `JsDate` の正規表現をそのまま移す（ISO の形と、legacy の `numeric`・`worded`）。`go_float` は Ruby の `Float#to_s` の最短の桁から指数を組み立てる。`JsonValue.parse` は `JSON.parse(text, allow_duplicate_key: true)`（gem が古くて引数がなければ外す）のあと `normalize`。
- [ ] **Step 4: テストを通す。** → PASS
- [ ] **Step 5: コミット**（`git commit -m "JS と同じ文字・日付・数の扱いを Rails 側に作る"`）

### Task 3: 単語リスト・パック・記録・ワークスペースの検査（`src/lib` の移植）

**Files:**
- Create（`rails/app/lib/typle/` に、Kotlin の同名のものを1ファイルずつ）: `app_error.rb`（`Typle::AppError`（`key`）・`Typle::PlainError`）、`limits.rb`、`devices.rb`、`interests.rb`、`word_lists.rb`、`records.rb`、`word_packs.rb`（`Typle::Library = Struct.new(:packs, :lists)` を含む）、`followed_packs.rb`、`catalog.rb`、`workspace_request.rb`（`Typle::WorkspaceChange = Struct.new(:lists, :packs, :devices, :interests)`、`Typle::WorkspaceDocument`、`Typle::WorkspaceDocuments.decode`）、`workspace_change.rb`（Kotlin の `Workspace.readWorkspaceChange`・`unstorable`・`encodeChange` と `RecordStore.recordRows`・`legacyId`・`withRecords`）
- Test: `rails/test/lib/library_test.rb`

**Interfaces:**
- Consumes: Task 2 の `Typle::Js`・`JsDate`・`JsonValue`。
- Produces（Kotlin と同じ名前を snake_case にする。値は Hash（キーは String、順序を保つ）と Array）:
  - `Typle::WordPacks.normalize_library(lists, packs) -> Library`、`.validate_library(library)`、`.repair_library`、`.convert_library`、`.own_list_count(library)`、`.followed_pack_count(packs)`
  - `Typle::FollowedPacks.follow_official(library, official) -> Library`、`.without_followed_words(library)`
  - `Typle::Catalog.official -> Array<OfficialPack>`（`OfficialPack = Struct.new(:catalog_id, :version, :pack, :lists)`）、`.followed(library)`。パックのデータは `ENV["TYPLE_APP_DATA"]`（既定 `Rails.root.join("..")`）の `src/lib/packs/{samples,countries}.json`・`commands/manifest.json` の順と、`src/messages/ja/format.json` の `defaultDevice` から作る。一度だけ作って持つ。
  - `Typle::Records.add_record(lists, devices, list_id, input) -> Added(lists, record, added)`、`.delete_record(lists, list_id, record_id) -> Array|nil`
  - `Typle::WorkspaceRequest.validate(body) -> [revision, WorkspaceChange]`
  - `Typle::WorkspaceChanges.read(fields) -> [:ok, revision, change] | [:refused, key]`、`.unstorable?(value)`（`!valid_encoding?` か `"\0"` を含む文字列・キー）、`.encode(change) -> Encoded(devices, interests, library)`（`library` は `lists`・`packs`・`rows`・`own`・`followed`・`size`）
  - `Typle::RecordRows.rows(lists) -> Array<RecordRow(list_id, record_id, position, at_ms, data_json)>`、`.legacy_id(list_id, position, date, time)`、`.with_records(lists, stored)`

- [ ] **Step 1: 失敗するテストを書く。** Kotlin の `LibTest.kt` の次のテストを、同じ fixture と同じ assert で Minitest に移す: limits（`limits.json`、`MAX_IMPORT_BYTES` は飛ばす）、default interests、workspace-put（`workspace-put.json`。`readWorkspaceChange` の結果が `invalidWorkspace` か `levelsConflict`）、invalidWorkspace の本文の一覧（孤立サロゲートと U+0000 を含む、`"😀"` は通る）、interests null と levels、packs（`tests/fixtures/packs/*.json`、2回目の normalize で変わらない）、follow（`tests/fixtures/follow/*.json`）、records（`tests/fixtures/records/*.json`、`listNotFound` は `recordListDeleted`）、record-operations、legacy id（`record-ids.json` の `ids`）、公式パック（10個より多い、2回 follow しても同じ）。比べるときは両方を `JsonValue.normalize` してから `assert_equal`。
- [ ] **Step 2: 動かして失敗を見る。** `bin/rails test test/lib/library_test.rb` → FAIL
- [ ] **Step 3: 実装する。** Kotlin の `lib/*.kt` と `Workspace.kt` の上の部分を、同じ順で Ruby に移す。`Undefined` は「Hash にキーがない」で表す（`hash.key?`）。`JsObject.without/with` は `except`・`merge` で、元を変えない。
- [ ] **Step 4: テストを通す。** → PASS
- [ ] **Step 5: コミット**（`git commit -m "単語リストとパックと記録の検査を Rails に移す"`）

### Task 4: ログインの Cookie と、取り消しの確認

**Files:**
- Create: `rails/app/lib/typle/login_settings.rb`（`Typle::LoginSettings`）、`rails/app/lib/typle/sessions.rb`（`Typle::Sessions`、`Typle::Session = Struct.new(:sub, :name, :image, :issued_at, :expires)`）、`rails/app/models/application_record.rb`、`rails/app/models/session_revocation.rb`
- Modify: `rails/app/controllers/application_controller.rb`（`current_owner` の `before_action` 用メソッド）
- Test: `rails/test/lib/sessions_test.rb`

**Interfaces:**
- Produces:
  - `Typle::LoginSettings.read(env = ENV) -> LoginSettings | nil`（`#origin`・`#session_key`・`#secure`・`#cookie_name`）。`.derive_key(secret, info)` は HKDF-SHA256（`OpenSSL::KDF.hkdf`、salt なし、32 バイト、info `typle session v1`）。
  - `Typle::Sessions.open(key, value, now = Time.now.to_f) -> Session | nil`（Kotlin の `Sessions.open` と同じ検査。base64url は厳密に読む）
  - `Typle::Sessions.cookie_value(header, name) -> String | nil`
  - `SessionRevocation`（表 `typle_session_revocations`、主キー `owner_id`）。`SessionRevocation.revoked_at_ms(owner) -> Integer | nil`
  - `ApplicationController#signed_in_owner -> String | nil`：取り消されていれば `401 loginRequired` と、Cookie を消す `Set-Cookie: <name>=; Path=/; Max-Age=0; HttpOnly[; Secure]; SameSite=Lax` を返して止める。取り消しの確認が失敗したら `503 databaseUnavailable`。

- [ ] **Step 1: 失敗するテストを書く。** Kotlin の `LibTest` の sessions.json のテストを移す（`cookies` だけ。`tokens` は使わない）。アプリの鍵で封じた値（`auth-answers.json` の `alice-app`）は Cookie として開かない。いじった Cookie（署名・本文の1文字違い、`=` 付き、3つに分かれたもの、空）は `nil`。`https://typle.example` で `__Host-typle-session`、`http://127.0.0.1:3101` で `typle-session`。
- [ ] **Step 2: 動かして失敗を見る。** → FAIL
- [ ] **Step 3: 実装する。** `cookie_value` は Kotlin の `Account.cookieValue` と同じ（最初の一致、JS の空白で trim、`"…"` を外す）。
- [ ] **Step 4: テストを通す。** → PASS
- [ ] **Step 5: コミット**（`git commit -m "ログインの Cookie を Rails で読む"`）

### Task 5: モデルと保存、コントローラー、DB のテスト

**Files:**
- Create: `rails/app/models/workspace.rb`（表 `typle_workspaces`、主キー `owner_id`）、`rails/app/models/practice_record.rb`（表 `typle_records`、主キー `[:owner_id, :list_id, :record_id]`）、`rails/app/lib/typle/workspace_store.rb`（`Typle::WorkspaceStore`、`Typle::Conflict`）、`rails/app/controllers/health_controller.rb`、`workspaces_controller.rb`、`records_controller.rb`、`rails/app/lib/typle/database_url.rb`
- Modify: `rails/config/routes.rb`（Task 1 の仮の行き先を差し替える）、`rails/config/database.yml`
- Test: `rails/test/integration/workspace_test.rb`、`rails/test/lib/database_url_test.rb`

**Interfaces:**
- Consumes: Task 3 の `WorkspaceChanges`・`RecordRows`・`Records`・`Catalog`、Task 4 の `signed_in_owner`。
- Produces:
  - `Typle::DatabaseUrl.config(url) -> Hash`（`database.yml` が使う。`sslmode=require`/`prefer` → `verify-full`、`sslrootcert` は `ENV["PGSSLROOTCERT"]` か `/etc/ssl/certs/ca-certificates.crt`、`pool: 5`、`idle_timeout: 30`、`connect_timeout: 5`、`variables: { statement_timeout: 5000 }`）。URL は `DATABASE_URL`、なければ `POSTGRES_URL`。テストは `TYPLE_TEST_DATABASE_URL`。
  - `Typle::WorkspaceStore.read(owner) -> Saved | nil`（`Saved = Struct.new(:lists, :packs, :devices, :interests, :revision)`）、`.write(owner, revision, change) -> Saved`（`Typle::Conflict`・`AppError` を投げる）、`.modify(owner) { |document| [document, changed, result] } -> [result, Saved]`、`.answer(saved, owner) -> Hash`
  - 文は Kotlin の `readSavedWorkspace`・`modifyWorkspace`・`writeWorkspace`・`syncRecords` と同じ: 読み込みは `Workspace.select(...)` に records の `jsonb_agg` の副問い合わせ、保存は `transaction` の中で、revision 0 なら `pg_advisory_xact_lock(hashtext('typle_workspaces:' || owner))` → `insert_all(..., unique_by: :owner_id, returning: :revision)`（`ON CONFLICT DO NOTHING`）→ 記録の行の削除、それ以外は数を数える SELECT → `where(owner_id:, revision:).update_all(COALESCE …)`。`syncRecords` は DELETE 1文（unnest の NOT EXISTS）、`PracticeRecord.upsert_all(rows, unique_by: …, on_duplicate: Arel.sql("… WHERE (…) IS DISTINCT FROM (…)"))` 1文（行が空なら出さない）、`count` 1文。`modify` は `Workspace.lock`（`FOR UPDATE`）。
  - コントローラーの動きは Kotlin の `answerWorkspace`・`answerRecords`・`readSaveRequest`・`changeRecordAnswer` と同じ順の検査: ログインの設定がない → `503 authUnavailable`、書き込みで `Origin` が違うか `Content-Type` の `;` の前が `application/json` でない → `403 forbiddenRequest`、ログイン、本文（空 → `400 emptyRequest`、1 MiB 超 → `413 workspaceTooLarge`、JSON でないかオブジェクトでない → `400 invalidWorkspace`／`invalidRecord`、`accountId` が違う → `401 loginAccountChanged`）。記録の追加は新しければ 201、同じ id なら 200、どちらも `record` を付ける。`rescue_from StandardError` で 503。
  - `HealthController#show`：`SELECT 1` が通れば `200 {"status":"ok"}`、だめなら `503 databaseUnavailable`。

- [ ] **Step 1: 失敗するテストを書く。** DB のないテスト（`TYPLE_TEST_DATABASE_URL` がなくても動く）: Kotlin の `RoutesTest` の「DB がないときの 503 と、書き込みを開く前に断る」（`Typle::WorkspaceStore` と health の DB 確認を `stub` で例外にする）と `DatabaseUrl` の変換（Neon の URL → `sslmode: "verify-full"`、`p%40ss` → `p@ss`）。DB のテスト（`skip unless ENV["TYPLE_TEST_DATABASE_URL"]`、ランダムな `github:<10桁>` の owner、`Minitest.after_run` で3表から消す、`self.use_transactional_tests = false`）: Kotlin の `DatabaseTest` の全部（workspace-writes.json・record-writes.json の各シナリオと `state(owner)` の比較、0 から読んで保存して読み戻す、409、拒否の一覧（アプリのトークンの行は外す）、取り消し、同じ revision の2つの保存はどちらか1つ（2スレッド）、health 200）。それに Review Focus の 2・4・5:

```ruby
test "integral floats are saved and answered as JS writes them" do
  body = %({"revision":0,"accountId":"#{owner}",#{library("one").sub('"records":[]', '"records":[{"id":"r","time":1500.0,"date":"2026-09-07T00:00:00.000Z","mistakes":0,"accuracy":100.0}]')}})
  put "/api/workspace", params: body, headers: headers_for(owner)
  assert_includes response.body, '"time":1500,'
  assert_equal 1500, PracticeRecord.where(owner_id: owner).pick(Arel.sql("data->'time'"))
end

test "a body of exactly 1 MiB is read, one byte more is 413" do
  put "/api/workspace", params: %({"x":"#{"a" * ((1 << 20) - 8)}"}), headers: headers_for(owner)
  assert_equal 400, response.status
  put "/api/workspace", params: %({"x":"#{"a" * ((1 << 20) - 7)}"}), headers: headers_for(owner)
  assert_equal 413, response.status
end

test "an unexpected error is a JSON 503 with the headers" do
  Typle::WorkspaceStore.stub(:write, ->(*) { raise "boom" }) do
    put "/api/workspace", params: %({"revision":0,"accountId":"#{owner}",#{library("one")}}), headers: headers_for(owner)
  end
  assert_equal 503, response.status
  assert_equal "{\"error\":\"workspaceSaveFailed\"}\n", response.body
  assert_equal "rails", response.headers["x-typle-backend"]
end
```

- [ ] **Step 2: 動かして失敗を見る。** `URL=$(grep '^DATABASE_URL=' ../.env.local | cut -d= -f2- | tr -d '"') TYPLE_TEST_DATABASE_URL=$URL bin/rails test` → FAIL
- [ ] **Step 3: 実装する。**
- [ ] **Step 4: テストを通す。** DB ありとなしの両方で `bin/rails test` → PASS（DB なしは DB のテストが skip）。
- [ ] **Step 5: 文の数を確かめる。** `ActiveSupport::Notifications.subscribe("sql.active_record")` で、GET・PUT（revision 0 と 1）・記録の追加で流れる文を数え、Kotlin と同じ数であることをテストに書く（SCHEMA と TRANSACTION の名前の文は数えない。初回のカラムの読み込みも数えない）。
- [ ] **Step 6: コミット**（`git commit -m "workspace と records の API を Rails で答える"`）

### Task 6: Next.js 側の振り分けと計測

**Files:**
- Modify: `scripts/backendCompare.cjs`、`scripts/bench-backends.cjs`、`.vercelignore`（`rails` を足す）、`eslint.config.mjs`・`knip.json`・`tsconfig.json`（`rails/` を見ないように。要るものだけ）
- Test: `tests/backendCompare.test.cjs`、`tests/benchBackends.test.cjs`

**Interfaces:**
- Produces: `railsOrigin(env) -> string | null`（`TYPLE_RAILS_URL`。検査は `kotlinOrigin` と共通の `serviceOrigin(name, env)` にまとめ、エラーの文は `TYPLE_RAILS_URL (…) は …` の形）。`backendRewrites` は比較モードのときだけ、Kotlin のルールのあとに `x-typle-backend: rails` → `${rails}${source}` を足す。bench の `TARGETS` は `['next', 'go', 'kotlin', 'kotlin-direct', 'rails', 'rails-direct']`、`--rails <URL>` を受け、`rails`・`rails-direct` は `--rails` があるときだけ測る。`rails-direct` は `--rails` の URL を直接叩き、期待するヘッダーは `rails`。

- [ ] **Step 1: 失敗するテストを書く。**

```js
test('with TYPLE_RAILS_URL x-typle-backend: rails goes to the same path on the Rails service', () => {
  const rules = backendRewrites({ TYPLE_BACKEND_COMPARE: '1', TYPLE_RAILS_URL: 'https://typle-rails-abc.a.run.app/' })
  const rails = rules.filter(rule => rule.has[0].value === 'rails')
  assert.deepEqual(rails.map(rule => rule.destination), COMPARED_PATHS.map(source => `https://typle-rails-abc.a.run.app${source}`))
  assert.equal(rules.length, COMPARED_PATHS.length * 2)
  assert.deepEqual(backendRewrites({ TYPLE_RAILS_URL: 'https://r.example' }), [])
})

test('TYPLE_RAILS_URL must be an https origin', () => {
  assert.equal(railsOrigin({}), null)
  for (const value of ['http://r.example', 'https://r.example/api', 'r.example']) {
    assert.throws(() => railsOrigin({ TYPLE_RAILS_URL: value }), /TYPLE_RAILS_URL/, value)
  }
})
```

`benchBackends.test.cjs` には、`--rails` がなければ `rails`・`rails-direct` を送らないことと、あれば `rails-direct` が `--rails` のオリジンに行くことを、今の Kotlin のテストと同じやり方で足す。
- [ ] **Step 2: 動かして失敗を見る。** `node --test tests/backendCompare.test.cjs tests/benchBackends.test.cjs` → FAIL
- [ ] **Step 3: 実装する。**
- [ ] **Step 4: テストを通す。** 同じコマンド → PASS。`npm run lint` と `npm run knip` も通る。
- [ ] **Step 5: コミット**（`git commit -m "x-typle-backend: rails を Rails のサービスに回し、計測に加える"`）

### Task 7: Docker・手順書・バージョン・verify

**Files:**
- Create: `rails/Dockerfile`、`rails/Dockerfile.dockerignore`、`rails/README.md`
- Modify: `docs/backend-compare.md`、`README.md`（文書の表と関連の説明）、`package.json`・`package-lock.json`（`npm version minor --no-git-tag-version`。番号は `git tag` と `gh release list` を見て、使われていないものに）

**Interfaces:**
- Consumes: すべて。

- [ ] **Step 1: Dockerfile を書く。** コンテキストはリポジトリのルート。`ruby:3.4.1-slim` で `bundle install`（`BUNDLE_WITHOUT=development:test`、`BUNDLE_DEPLOYMENT=1`）、`src/lib/packs` と `src/messages/ja/format.json` を `/app-data/src/...` に写し `TYPLE_APP_DATA=/app-data`、`RAILS_ENV=production`、`bootsnap precompile`、`SECRET_KEY_BASE_DUMMY=1` か起動時のランダムな値、`CMD bin/rails server -b 0.0.0.0 -p ${PORT:-8080}`。`Dockerfile.dockerignore` は Kotlin のものにならって、ビルドが読むものだけを通す。
- [ ] **Step 2: イメージを手元で確かめる。** `docker build -f rails/Dockerfile -t typle-rails .` のあと、`docker run --rm -p 8082:8080 -e TYPLE_BACKEND_COMPARE=1 -e AUTH_SECRET=s -e AUTH_URL=http://127.0.0.1:3101 -e DATABASE_URL=<host.docker.internal に直した .env.local の URL> typle-rails`、`curl -si 127.0.0.1:8082/api/health` → `200`、`x-typle-backend: rails`。`TYPLE_BACKEND_COMPARE` なしで `404`。
- [ ] **Step 3: `rails/README.md` を書く。** Kotlin の README と同じ項目（受けるパス、環境変数、コードの構成、DB の接続、テストの動かし方（手元の Ruby と Docker）、ビルドと手元での実行、Cloud Run へのデプロイ（Kotlin と同じフラグ: 1 vCPU・2 GiB・min 0・同時実行数 80・`--cpu-boost`・`--no-cpu-throttling` なし・Secret Manager）、消し方）と、「Next.js と違うところ」（JSON のバイトの違い、トークンを読まないこと、Puma の5スレッドと同時実行数80）。
- [ ] **Step 4: 文書を直す。** `docs/backend-compare.md` を4つのバックエンドに（表、`TYPLE_RAILS_URL`、`--rails`、比べ終わったら消すもの）。README の文書の表の `backend-compare.md` の説明。
- [ ] **Step 5: バージョンを上げて verify。** `npm run verify`（サンドボックスの外で）→ 全部通る。`cd rails && bin/rails test`（DB あり）→ 全部通る。
- [ ] **Step 6: コミットして PR。** 件名は `Rails のバックエンドを速度比較に加える (vX.Y.Z)`。
