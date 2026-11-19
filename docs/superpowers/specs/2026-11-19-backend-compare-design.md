# Next.js・Go・Kotlin のバックエンドの速度比較（設計）

日付: 2026-11-19 ／ 対象: typle-r（main は v2.51.1）

## 目的

いまの Next.js の Route Handler と、v2.47.0 で消した Go のバックエンド、新しく書く Kotlin のバックエンドの速さを、本番に近い環境で同じ条件で比べる。比べるためだけの仕組みで、Go や Kotlin を本番のバックエンドにするものではない。

## 目標

- 環境変数 `TYPLE_BACKEND_COMPARE=1` を付けた Vercel のデプロイ（プレビュー）では、3つのバックエンドがすべて動く。
- そのデプロイでは、リクエストヘッダー `x-typle-backend` で行き先を選ぶ。
  - `go` なら Vercel の Go 関数に行く。
  - `kotlin` なら Cloud Run の Kotlin に行く。
  - ヘッダーがないときと `next` なら、これまでどおり Next.js に行く。
- 計測スクリプトを用意する。同じ時間帯に3つを交互に叩き、各エンドポイントの p50・p95・最大の往復時間を出す。Kotlin は、Cloud Run を直接叩いた時間も別の行に出す。

## 目標にしないこと

- Go や Kotlin を本番で使うこと。
- Go や Kotlin を今後の Next.js の変更に追随させること。比べたい時点の動きがそろっていれば足りる。
- auth（signin・callback・session・token・signout・local）を Go や Kotlin で受けること。
- 前の typle-kotlin-backend リポジトリ（v2.6.10 でアーカイブ）を復活させること。API が v2.6 のころのままなので使わない。

## 失敗とみなすこと（動いても駄目なもの）

- 本番（環境変数なし）で Go や Kotlin に届く道があること。本番で `x-typle-backend` を付けても、何も変わってはいけない。
- 比較が公平でないこと。たとえば、片方だけ毎回コールドスタートを踏む、片方だけ DB の接続を使い回さない、片方だけ余計な処理をする、など。Kotlin が転送1回ぶん不利になるのは避けられないので、直接叩いた数字を並べて見えるようにする。
- 本番のデータを汚すこと。計測で書き込むのは、専用のテスト用アカウントだけにする。

## 対象のエンドポイント

| パス | メソッド |
|---|---|
| `/api/health` | GET |
| `/api/workspace` | GET、PUT |
| `/api/workspace/records` | POST、DELETE（いまの Next.js と同じもの） |

## 今の作り（調べた結果）

- `/api/*` は `src/app/api` の Route Handler 10本だけで受けている。`src/proxy.ts` は matcher で `/api` を外していて、ページの CSP の nonce を付けるだけ。
- Vercel のデプロイは CLI だけで行う（`vercel.json` の `git.deploymentEnabled: false`）。
- v2.46.11（`6b85a4c2`）の Go は次の作り。
  - `api/index.go`（Vercel の Go 関数1本）→ `server/vercel` → `server/internal/{web,workspace,store,session,auth,serve}`。
  - `go.mod` はリポジトリのルートにある。
- セッションは Go（`server/internal/session`）と TS（`src/server/session.ts`）で同じ形式。どちらも `AUTH_SECRET` から種類ごとの鍵を作り、HMAC-SHA256 で署名する。ログインした Cookie は、Go でもそのまま読める。Kotlin も同じ形式で読む。
- 共通の fixture が `tests/fixtures/` にある（packs・follow・default-interests など）。前の Kotlin 版もこれの写しでテストしていた。
- 保存の計測（`docs/save-metrics.md`）は、`typle_save_metrics` にハンドラーの中でかかった時間を残している。

## 選んだやり方

### 検討した案

切り替え方:

1. **環境変数で、デプロイごとに1つだけ使う**: 切り替えのたびに再デプロイが要る。別々の時間に測ることになり、条件の差が数字に混ざる。
2. **環境変数で比較モードをオンにし、リクエストごとに選ぶ（採用）**: 同じデプロイで交互に叩けるので、公平に比べられる。
3. 手元だけで比べる（外した案）: Vercel のコールドスタートや DB までの距離が入らない。ユーザーは Vercel の上での比較を選んだ。

Kotlin の置き場所（Vercel では JVM を動かせない）:

- **Google Cloud Run（採用）**: コンテナで置けて、使わないときは 0 台まで減るので、Vercel の関数に近い条件にできる。
- AWS Lambda: JVM のコールドスタートが大きく、SnapStart などで条件をそろえる手間が増える。一度選んだが、ユーザーが Cloud Run に戻した。
- Kotlin/JS にして Vercel で動かす: 中身が Node なので、JVM の比較にならない。

ユーザーは、切り替えは 2、置き場所は Cloud Run を選んだ。

### 振り分け

- `next.config.mjs` の `rewrites().beforeFiles` にルールを足す。ルールは `TYPLE_BACKEND_COMPARE=1` のビルドでだけ出す。
  - `x-typle-backend: go` → 対象パスを Go 関数（`/api/go/:path*` など）に回す。
  - `x-typle-backend: kotlin` → 対象パスを `TYPLE_KOTLIN_URL`（Cloud Run のサービスの URL）の同じパスに回す。
- rewrites で足りなければ、`src/proxy.ts` の matcher に対象パスを足し、そこで `NextResponse.rewrite` してもよい（実装する人の判断）。
- Go と Kotlin は、元の URL のパス（`/api/workspace` など）で振り分ける。
- Kotlin への転送では、Cookie と `x-typle-backend` 以外のヘッダーもそのまま渡す。Kotlin がクライアントの IP を使う処理はないので、転送元の IP の扱いは気にしなくてよい。

### 本番で閉じる

- Go 関数は、実行時の環境変数 `TYPLE_BACKEND_COMPARE` が `1` でなければ、すべてのリクエストに `404` を返す。本番に Go のファイルが載っていても、`/api/go/...` を直接叩いても使えない。
- Vercel の本番の環境変数には、`TYPLE_BACKEND_COMPARE` と `TYPLE_KOTLIN_URL` を付けない。Preview 環境にだけ付ける。
- Cloud Run のサービスは公開 URL で受ける。ただし動きは本番の API と同じで、ログインの Cookie かトークンがなければ何も読めず、書けない。比較が終わったら、サービスを消すか 0 台に止める手順を README に書く。

### どれが答えたかを分かるようにする

- 対象のエンドポイントは、レスポンスヘッダー `x-typle-backend` に `next`・`go`・`kotlin` のどれかを入れて返す。Next.js 側は、比較モードのときだけ付ければよい。
- 計測スクリプトは、このヘッダーが期待どおりでないリクエストを数に入れず、エラーとして報告する。

### Go のコード

- v2.46.11 の Go を戻す。残すのは、対象のエンドポイントと、Cookie とトークンのセッション確認に要る部分だけ。
- 消すのは、管理画面・共有・ギャラリー・通報・auth のハンドラーと、それだけが使う store の処理。migration 0036〜0038 で消したテーブルを触るため。
- workspace と records の動き（入力の検査、revision の楽観ロック、`409`・`413` など）は、いまの Next.js にそろえる。v2.46.11 のあとに Next.js 側で変わったところがあれば、Go を直す。
- そろっていることは、共通 fixture（`tests/fixtures/`）と `go test` で確かめる。
- ディレクトリは戻したもの（`server/`・`api/`）と同じでよい。`.vercelignore` の `server/cmd` と `*_test.go` も戻す。
- 保存の計測（`typle_save_metrics`）は Go では残さない。比べるのはクライアントから見た往復時間。

### Kotlin のコード

- typle-r の中に新しく書く（例: `kotlin/`）。Ktor の HTTP サーバーで、Cloud Run には Docker イメージで載せる。JDK 21。
- 受けるのは対象のエンドポイントだけ。動きは Next.js にそろえ、共通 fixture（`tests/fixtures/`）を直接読むテストで確かめる。写しは作らない。
- DB は Next.js・Go と同じ Postgres に、接続プールを使ってつなぐ。比べるのは処理の速さなので、プールの大きさなどは Go と Next.js にそろえる。
- 起動の速さも比べる対象なので、GraalVM のネイティブイメージや CRaC は使わず、ふつうの JVM で動かす。
- Cloud Run の設定:
  - リージョンは、Neon の DB に一番近いところにする。Vercel の関数のリージョンも同じ近さかを確かめ、README に書く。
  - 最小インスタンスは 0。
  - 計算資源（Claude の判断）: Vercel の関数は既定のまま（Fluid compute がオン、Standard の 1 vCPU・2 GB）。Cloud Run はこれに合わせて 1 vCPU・2 GiB にし、同時実行数は既定の 80、CPU はリクエストの間だけ割り当て、起動時の CPU ブーストはオンにする。ブーストはオフにすると JVM に不利すぎて、ふつうの使い方からずれるため。設定は README に書き、計測の結果と一緒に残す。
  - `AUTH_SECRET` と `DATABASE_URL` は Secret Manager から渡す。
  - デプロイは `gcloud` で、ユーザーの Mac から手で行う。コマンドを README（`kotlin/README.md`）に書く。
- `.vercelignore` に `kotlin/` を足して、Vercel には載せない。

### 計測スクリプト

- `scripts/bench-backends.cjs`（名前は実装する人が決めてよい）。引数で、デプロイの URL、Cloud Run の URL、回数、ログイン Cookie を受け取る。
- Cookie は、ブラウザーでテスト用アカウントにログインしてコピーしたものを、環境変数か引数で渡す。スクリプトは `AUTH_SECRET` を使って Cookie を作らない。
  - プレビューは本番と同じ Neon を使う（ユーザーの答え）。プレビューでは OAuth の戻り先がないので、本番（`typle.sparklingstadt.com`）でログインした Cookie の値をそのまま使う。スクリプトが `Cookie` ヘッダーを自分で付けるので、ホストが違っても送れる。プレビューの `AUTH_SECRET` が本番と違って `401` になるときは、スクリプトがそう表示して止まる（Claude の判断）。
  - 開発用のログイン（`/api/auth/local`）はプレビューでもオンにしない。本番の DB に架空のアカウントが入るため（Claude の判断）。
  - Deployment Protection がかかっているときのために、`VERCEL_AUTOMATION_BYPASS_SECRET` があれば `x-vercel-protection-bypass` ヘッダーを付ける。
  - プレビューのビルドの `npm run db:migrate` は本番の DB にかかるが、この変更はマイグレーションを足さないので何も起きない。
- 測り方:
  - 各エンドポイントについて、まず全部を数回ずつ叩いて温める（結果は捨てる）。
  - そのあと、`next` → `go` → `kotlin`（Vercel 経由）→ `kotlin`（直接）の順に、N 回（既定 50）くり返し叩く。
  - p50・p95・最大と、失敗の数を表で出す。
- PUT は、GET で読んだ workspace を、revision を付けてそのまま書き戻す（中身は変えない）。Next.js の `writeWorkspace` は中身が同じでも行を書き換えて revision を1つ上げるので、これで本当の書き込みを測れる。Go と Kotlin も同じ動きにそろえる。records は1件足して、すぐ消す。
- コールドスタートも見たいときのために、温めを飛ばすオプションを付ける。

## 制約

- リポジトリの決まり（`CLAUDE.md`・`docs/release-workflow.md`）に従う。新しいバージョンを付けた PR、`npm run verify`、日本語の CHANGELOG と docs。
- `npm run verify` に `go test` と Kotlin のテスト（`./gradlew test`）を足すかは、実装する人が決めてよい。足さない場合は、それぞれの動かし方を README に書く。
- Vercel のプレビューは CLI の `vercel deploy`（`--prod` なし）で出す。Cloud Run のデプロイも、クラウドのセッションには資格情報がないので、ユーザーの Mac で行う。
- Go のバージョンは、v2.46.11 の `go.mod` に合わせる（Go 1.25）。

## 実装する人に任せること

- 振り分けを rewrites で書くか、proxy で書くか。
- Go の中で消すものと残すものの細かい線引き。上の対象を満たし、消したテーブルに触らなければよい。
- Kotlin のライブラリ（DB ドライバー、JSON、接続プール）とディレクトリの作り。
- 計測スクリプトの引数と出力の形。
- テストの粒度。少なくとも次は確かめる。
  - 比較モードでないときに、Go と Kotlin に届かないこと。
  - ヘッダーで振り分け先が変わること。
  - 3つが共通 fixture で同じ結果を返すこと。
