# Rails のバックエンドを速度比較に加える（設計）

日付: 2026-11-21 ／ 対象: typle-r（main は v2.56.x）

## 目的

いまの速度比較（[docs/backend-compare.md](../../backend-compare.md)、設計は [2026-11-19-backend-compare-design.md](2026-11-19-backend-compare-design.md)）に、4つめのバックエンドとして Ruby on Rails を加える。Next.js・Go・Kotlin と同じ条件で、同じエンドポイントの往復時間を比べる。比べるためだけのもので、Rails を本番のバックエンドにはしない。

ユーザーが決めたこと:

- 比べるのは速度。
- 形は Kotlin と同じ（リポジトリの中のディレクトリー、Cloud Run に置く、`x-typle-backend` で振り分ける、共通 fixture で確かめる）。
- 書き方は Rails らしくてよい（ActiveRecord・コントローラー・`config/routes.rb` を使う。Kotlin のように JS の JSON の読み書きまで手で移植することはしない）。

## 目標

- `TYPLE_BACKEND_COMPARE=1` と `TYPLE_RAILS_URL` を付けた Vercel のプレビューでは、`x-typle-backend: rails` のリクエストが Cloud Run の Rails に届く。
- Rails は `/api/health`（GET・HEAD）、`/api/workspace`（GET・HEAD・PUT）、`/api/workspace/records`（POST・DELETE）に、Next.js・Kotlin と同じ状態コード・同じ JSON の値・同じ DB の結果で答える。
- `scripts/bench-backends.cjs` に `--rails <URL>` を足し、`rails`（Vercel 経由）と `rails-direct`（Cloud Run を直接）の行を出す。

## 目標にしないこと

- 本番で Rails を使うこと。`TYPLE_BACKEND=rails` の切り替え（[docs/backend-switch.md](../../backend-switch.md) の Go のようなもの）は作らない。
- ログイン（Auth.js の `/api/auth/*`）と `/api/account/*` を Rails で受けること。Rails は Cookie を読むだけ。
- 応答のバイト列を Next.js とそろえること。そろえるのは JSON としての値（Go・Kotlin と同じ基準）。
- 今後の Next.js の変更に Rails を追随させること。
- Rails のマイグレーションで DB の形を持つこと。表の形は今までどおり `db/` の SQL が持つ。

## 失敗とみなすこと

- 本番（環境変数なし）で Rails に届く道があること。Rails 自身も `TYPLE_BACKEND_COMPARE=1` がなければ全部 `404 {"error":"notFound"}`。
- 比較が公平でないこと。Rails だけ DB の接続を使い回さない、Cloud Run の資源が Kotlin と違う、開発モードで動く、など。
- 本番のデータを汚すこと。書き込むのは計測用のテストアカウントだけ。
- Rails の答えが fixture と食い違ったまま計測すること（遅い・速いが、違う仕事をした結果になる）。

## 今の作り（調べた結果）

- 振り分けは `scripts/backendCompare.cjs` の `backendRewrites`。`TYPLE_KOTLIN_URL` を https のオリジンだけと確かめ、`x-typle-backend: kotlin` を beforeFiles の rewrite で転送する。テストは `tests/backendCompare.test.cjs`・`backendPaths.test.cjs`・`benchBackends.test.cjs`。
- Kotlin（`kotlin/`、約 3,000 行）は、`src/server/workspace.ts`・`records.ts`・`account.ts`・`session.ts` と、それが使う `src/lib`（wordLists・wordPacks・followedPacks・devices・interests・records・workspaceRequest・packs のカタログ）の移植。テストは `../tests/fixtures` を直接読み、DB のテストは `TYPLE_TEST_DATABASE_URL` があるときだけ動く。`npm run verify` には入れていない。
- 表は `typle_workspaces`（owner_id が主キー、lists・packs・devices・interests が jsonb、revision、records_split）、`typle_records`（owner_id・list_id・record_id・position・at・data）、`typle_session_revocations`（owner_id・revoked_at）。
- 手元には Ruby 3.4.1（rbenv）と Rails 8.1.4 がある。

## 選んだやり方

### 検討した案

1. **Rails の API モード（`rails new --api`）で、ActiveRecord とコントローラーで書く（採用）**: ふつうの Rails の重さ（ミドルウェア・ActiveRecord）がそのまま数字に出るので、「Rails で書いたら速さはどうか」の答えになる。ユーザーの「Rails らしい書き方でいい」に合う。
2. Rails の中で生の SQL と自前の JSON 読み書きを使い、Kotlin を行ごとに移す: 速いかもしれないが、もはや Rails らしいコードの速さではない。
3. Sinatra や Roda などの軽い Ruby: Rails の比較にならない。

### 置き場所と構成

- `rails/` に `rails new rails --api --database=postgresql --skip-git --skip-docker` 相当で作り、要らないもの（Action Mailer・Active Job・Active Storage・Action Cable・Solid 系・Kamal・Thruster）は入れない。Dockerfile は Kotlin にならって自分で書く。
- Ruby 3.4.1、Rails 8.1、Puma。YJIT はオン（Rails 8 の既定）。本番モード（`RAILS_ENV=production`）で動かし、`eager_load` する。
- Puma は 1 プロセス・5 スレッド、ActiveRecord の接続プールも 5（Next.js の `src/server/database.ts` の最大 5 本にそろえる）。アイドルの接続は 30 秒で閉じ、接続とクエリは 5 秒で諦める（`connect_timeout`・`statement_timeout`）。`sslmode=require` は `verify-full` として扱う（Kotlin と同じ。node-postgres 8 の解釈）。
- `DATABASE_URL`（なければ `POSTGRES_URL`）・`AUTH_SECRET`・`AUTH_URL`・`TYPLE_BACKEND_COMPARE`・`PORT` を読む。Rails の `SECRET_KEY_BASE` は使わないので、起動に要るなら適当な値をイメージの中で作る（Cookie の署名には使わない）。
- 公式パックのデータは写しを作らず、`../src/lib/packs` を読む。Docker のビルドはリポジトリのルートをコンテキストにして、`src/lib/packs` と `src/messages/ja/format.json`（Kotlin が使っているもの）をイメージに写す。置き場所は環境変数か設定 1 か所で決める。

### Rails の中の作り

- `config/routes.rb` に3つのパスとメソッドを書く。HEAD は Rails の GET の扱いに任せる。ほかのメソッドは、パスごとの catch-all で `405`（`Allow` は Next.js と同じ並び）。知らないパスは `404 {"error":"notFound"}`。
- 比較モードのゲート、4つのセキュリティヘッダーと `/api` の CSP、`x-typle-backend: rails` は、Rack のミドルウェア1つで付ける（404・405 にも付くように）。
- モデル: `Workspace`（`typle_workspaces`、主キー `owner_id`）、`PracticeRecord`（`typle_records`。`Record` は紛らわしいので避ける）、`SessionRevocation`（`typle_session_revocations`）。
- 楽観ロック（revision）、`FOR UPDATE`、advisory lock、records の upsert（`IS DISTINCT FROM` で変わった行だけ書く）は、Next.js と同じ SQL の意味を保つ。ActiveRecord（`lock`・`upsert_all`・`transaction`）で書けるところはそう書き、書けないところはモデルのメソッドの中の SQL にする。DB に流れる文の数と種類は Next.js と同じにする（ここが速さに効くので、ActiveRecord の都合で問い合わせを増やさない）。
- コントローラー: `HealthController`、`WorkspacesController`（show・update）、`RecordsController`（create・destroy）。ログインの確認は `ApplicationController` の `before_action`。
- 入力の検査とワークスペースの組み立て（`src/lib` の移植）は、`app/models` か `lib/typle/` のふつうの Ruby のクラスにする。JSON は Ruby の `JSON` で読み、`render json:` で返す。
- ログインの Cookie（`src/server/session.ts`: HKDF の info `typle session v1`、HMAC-SHA256）は OpenSSL で読む。Rails の `cookies.signed` は使わない（形が違う）。`typle_session_revocations` で取り消されたログインは `401`、ブラウザーの Cookie は消す（Next.js と同じ）。
- アプリのトークン（`Authorization: Bearer`、HKDF の info `typle app session v1`）は読まない。いまの Next.js（v2.55.0 から）が読まないため。`Authorization` ヘッダーは無視し、Cookie だけで決める。Kotlin は v2.55.0 より前に書いたのでトークンを読むが、Rails はいまの Next.js に合わせる。`tests/fixtures/auth-answers.json` のトークンの例は Rails のテストでは使わない（README に書く）。
- 書き込みの `Origin` の確認、本文の大きさの上限（`413`）、`409` の衝突は Next.js と同じ。
- 保存の計測（`typle_save_metrics`）は書かない（Go・Kotlin と同じ）。

### JSON の違いの扱い

Ruby の `JSON` は JS の `JSON.parse` と細部が違う（同じキーが2回出たとき、`-0`、大きな数、孤立サロゲートなど）。fixture にある例はすべて Next.js と同じ値にする。fixture にない細部の違いは、見つけたものを `rails/README.md` に書いて残す。Kotlin のように JS と同じ JSON パーサーは書かない。

### Next.js 側

- `scripts/backendCompare.cjs`: `TYPLE_RAILS_URL` を `TYPLE_KOTLIN_URL` と同じ検査で読み、比較モードのときだけ `x-typle-backend: rails` を転送する rewrite を足す。検査は Kotlin と共通の関数にまとめる。`TYPLE_BACKEND` に `rails` は足さない。
- `scripts/bench-backends.cjs`: `--rails <URL>` を足し、1回ごとに `next` → `go` → `kotlin` → `kotlin-direct` → `rails` → `rails-direct` の順で送る。`rails` と `rails-direct` の行は、`--rails` を渡したときだけ測る（`TYPLE_RAILS_URL` のないプレビューで `rails` を送ると Next.js が答え、全部失敗になるため。Claude の判断）。Kotlin の行の扱いは今のまま変えない。
- テスト: `tests/backendCompare.test.cjs`・`benchBackends.test.cjs` に Rails の場合を足す（URL がないと rewrite が出ない、比較モードでないと出ない、不正な URL でビルドが失敗する、ヘッダーで振り分ける）。
- `.vercelignore` に `rails` を足す。ESLint・knip・TypeScript が `rails/` を見ないようにする（要れば）。

### Cloud Run

Kotlin と同じ設定にする: リージョンは Neon の DB に一番近いところ、1 vCPU・2 GiB、最小インスタンス 0、CPU はリクエストの間だけ、起動時の CPU ブーストあり、`AUTH_SECRET` と `DATABASE_URL` は Secret Manager から。同時実行数も Kotlin と同じ 80 にする（計測は1本ずつ送るので、Puma が5スレッドでも差は出ない。README に書く）。デプロイは `gcloud` でユーザーの Mac から手で行い、コマンドは `rails/README.md` に書く。比べ終わったらサービスを消す手順も書く。

### テスト

- Minitest（Rails の既定）。`../tests/fixtures` を直接読む。Kotlin の `LibTest.kt`・`ServerTest.kt` が確かめていることを同じだけ確かめる。少なくとも次の3つ。
  - 比較モードでなければ全部 `404`。405 と `Allow` が正しい。
  - `workspace-writes.json`・`record-writes.json` のシナリオ。保存の衝突、取り消されたログイン。
  - DB がないときの health の `503` と、開く前に書き込みを断ること。
- DB のテストは `TYPLE_TEST_DATABASE_URL` があるときだけ動き、ランダムな owner で行を足して最後に消す（Kotlin と同じ）。
- 動かし方は手元の Ruby で `cd rails && bundle install && bin/rails test`。Ruby がない人向けに Docker で動かすコマンドも README に書く。`npm run verify` には入れない（Kotlin と同じ。Ruby を verify の前提にしないため）。

### 文書とバージョン

- `rails/README.md`（Kotlin の README と同じ項目）、`docs/backend-compare.md` を4つのバックエンドの文書に直す、README の文書の表を直す。
- 新しいマイナーバージョンの PR にする（`docs/release-workflow.md` の手順。番号は PR を作るときに `git tag` と Releases を見て決める）。

## 制約

- リポジトリの決まり（`CLAUDE.md`）: 日本語の文書・コミット、PR、`npm run verify` を通す、main に直接 push しない。
- Cloud Run とプレビューのデプロイ、計測はユーザーの Mac で手で行う。この作業ではコードと手順書までを作る。

## 実装する人に任せること

- `app/` の中のクラスの分け方とファイル名、入力検査のクラスの置き場所。
- ActiveRecord で書くか SQL で書くかの線引き（上の「文の数と種類を変えない」を守る範囲で）。
- Dockerfile のベースイメージ（`ruby:3.4.1-slim` など）と、gem の固定のしかた。
- テストの細かい分け方。
