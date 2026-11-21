# バックエンドの速度比較（Next.js・Go・Kotlin・Rails）

いまの Next.js の Route Handler と、Go（v2.47.0 で消したものを比較用に戻したもの）、Kotlin（Ktor）、Rails の速さを、Vercel の上で同じ条件で比べる仕組みです。比べるためだけのもので、本番では Next.js しか動きません（本番でも Go に切り替えるときは [docs/backend-switch.md](backend-switch.md)）。設計は [docs/superpowers/specs/2026-11-19-backend-compare-design.md](superpowers/specs/2026-11-19-backend-compare-design.md) にあり、Rails は [docs/superpowers/specs/2026-11-21-rails-backend-design.md](superpowers/specs/2026-11-21-rails-backend-design.md) で足しました。

## しくみ

| バックエンド | 置き場所 | コード |
|---|---|---|
| Next.js | Vercel（いつもの Route Handler） | `src/app/api` |
| Go | Vercel の Go 関数 `api/go.go`（`/api/go`） | `api/`・`server/`・`go.mod` |
| Kotlin | Google Cloud Run | `kotlin/`（[kotlin/README.md](../kotlin/README.md)） |
| Rails | Google Cloud Run | `rails/`（[rails/README.md](../rails/README.md)） |

- 比べるのは `/api/health`（GET）、`/api/workspace`（GET・PUT）、`/api/workspace/records`（POST・DELETE）です。ログインまわりは Next.js だけです。
- 環境変数 `TYPLE_BACKEND_COMPARE=1` を付けてビルドしたデプロイだけが、リクエストヘッダー `x-typle-backend: go` を Go 関数に、`x-typle-backend: kotlin` を `TYPLE_KOTLIN_URL` の Cloud Run に、`x-typle-backend: rails` を `TYPLE_RAILS_URL` の Cloud Run に回します（`next.config.mjs` の rewrites。中身は `scripts/backendCompare.cjs`）。ヘッダーがないときと `next` は Next.js です。
- 比較モードでは、4つとも応答のヘッダー `x-typle-backend` に自分の名前（`next`・`go`・`kotlin`・`rails`）を入れます。
- 本番で閉じる:
  - 環境変数のないビルドには rewrites がないので、ヘッダーを付けても何も変わりません。
  - Go 関数と Kotlin と Rails は、実行時に `TYPLE_BACKEND_COMPARE=1` がなければ、すべてに `404` を返します。
  - Vercel の本番の環境変数には `TYPLE_BACKEND_COMPARE`・`TYPLE_KOTLIN_URL`・`TYPLE_RAILS_URL` を付けません。Preview にだけ付けます。
- 4つとも、本番と同じ Neon の DB を読み書きします。プレビューのビルドの `npm run db:migrate` も本番の DB にかかります（この仕組みはマイグレーションを足しません）。

## 比べる手順

1. Kotlin と Rails を Cloud Run に載せます（[kotlin/README.md](../kotlin/README.md)・[rails/README.md](../rails/README.md)）。リージョンは Neon の DB に一番近いところにします。出てきたサービスの URL を控えます。どちらか片方だけでも比べられます。
2. Vercel の Preview 環境に、環境変数を足します。

   ```bash
   vercel env add TYPLE_BACKEND_COMPARE preview
   ```

   ```bash
   vercel env add TYPLE_KOTLIN_URL preview
   ```

   ```bash
   vercel env add TYPLE_RAILS_URL preview
   ```

   値は `1` と、それぞれの Cloud Run の URL（`https://….run.app`）です。載せていないほうの URL は足しません。
3. このブランチをプレビューに出します（`--prod` は付けません）。

   ```bash
   vercel deploy
   ```

4. 本番（`https://typle.sparklingstadt.com`）に、計測用のテストアカウントでログインします。リストを1つ以上作っておき、ブラウザーの開発者ツールで `Cookie` ヘッダーの値をコピーします。プレビューには OAuth の戻り先がないので、本番の Cookie を使います。プレビューの `AUTH_SECRET` が本番と違うと `401` になり、スクリプトがそう表示して止まります。
5. 計測します。

   ```bash
   TYPLE_BENCH_COOKIE='<コピーした Cookie>' node scripts/bench-backends.cjs <プレビューの URL> --kotlin <Kotlin の URL> --rails <Rails の URL> --origin https://typle.sparklingstadt.com
   ```

   - `rails` と `rails-direct` は、`--rails` を渡したときだけ測ります（`TYPLE_RAILS_URL` のないプレビューに `rails` を送ると Next.js が答え、全部が失敗になるため）。`kotlin-direct` は `--kotlin` を渡したときだけです。

   - `--origin` は、バックエンドに渡した `AUTH_URL` のオリジンです。書き込みはこの `Origin` で送ります。違うと `403 forbiddenRequest` になります。
   - Deployment Protection がかかっているときは、`VERCEL_AUTOMATION_BYPASS_SECRET` を一緒に渡します。
   - `--rounds`（既定 50）、`--warmup`（既定 3。`0` にするとコールドスタートも入ります）、`--only health,workspace.get,workspace.put,records` で絞れます。

## 計測の中身

- 温めのあと、1回ごとに `next` → `go` → `kotlin`（Vercel 経由）→ `kotlin-direct`（Cloud Run を直接）→ `rails`（Vercel 経由）→ `rails-direct`（Cloud Run を直接）の順に、同じリクエストを送ります。同じ時間帯・同じ DB で比べるためです。
- 時間は、送ってから本文を読み終えるまでです。`x-typle-backend` が期待どおりでない応答と、失敗した応答は数に入れず、失敗として数えます。
- `workspace.put` は、GET で読んだ workspace を revision を付けてそのまま書き戻します。中身が同じでも行を書き換えて revision を上げるので、本当の書き込みを測れます（GET の時間は入りません）。
- `records` は、1件足して（POST）すぐ消す（DELETE）までの合計です。テスト用アカウントの最初のリストを使います。
- Kotlin と Rails は Vercel から Cloud Run への転送が1回余分に入ります。その分は `kotlin` と `kotlin-direct`、`rails` と `rails-direct` の差で見ます。
- 計算資源: Vercel の関数は既定のまま（1 vCPU・2 GB）、Cloud Run は 1 vCPU・2 GiB、同時実行数 80、CPU はリクエストの間だけ、起動時の CPU ブーストあり。結果を残すときは、この設定も一緒に残します。

## Go と Kotlin と Rails の動き

どれも、比べる時点の Next.js（`src/server/workspace.ts` ほか）に動きをそろえ、共通の fixture（`tests/fixtures/`）で確かめています。Go と Kotlin と Rails を、これからの Next.js の変更に合わせ続けることはしません。比べ直すときに食い違っていたら、そのときに直します。

- Go は v2.46.5 のコード（workspace と records のハンドラーがある最後の版）から戻し、管理画面・共有・ギャラリー・ログイン・保存の計測を消しました。
  - `server/internal/workspace/packs/` は `src/lib/packs` の公式パックの写しです。`scripts/generate-packs.cjs` は書かないので、カタログが変わったら手で写します。違えば `go test` の `TestPacksMatchApp` が失敗します。
  - 手元では `go run ./server/cmd/typle-web` で `127.0.0.1:8081` に立ちます（比較モードのゲートはありません）。
  - テストは `go vet ./...` と `go test ./...` です。DB のテストは `TYPLE_TEST_DATABASE_URL` に `.env.local` の `DATABASE_URL` を入れると動きます。
- Kotlin のビルドとテストは [kotlin/README.md](../kotlin/README.md) を見てください。
- Rails は Rails らしい書き方（ActiveRecord・コントローラー・`config/routes.rb`）で書き、DB に流す文の数と種類だけは Next.js と同じにしています（v2.57.0 から）。ビルドとテストは [rails/README.md](../rails/README.md) を見てください。
- JSON としての値は4つとも同じですが、バイトの細部は違うことがあります。たとえば Go は保存された lists をそのまま返すので、`2.50` の末尾のゼロが残ります。

## 比べ終わったら

- Vercel の Preview 環境から `TYPLE_BACKEND_COMPARE`・`TYPLE_KOTLIN_URL`・`TYPLE_RAILS_URL` を消します。
- Cloud Run のサービスを消します（[kotlin/README.md](../kotlin/README.md)・[rails/README.md](../rails/README.md)）。
- テスト用アカウントは、計測で足した記録を消し終えています。いらなければアカウントごと消します。
