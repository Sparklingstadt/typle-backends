# バックエンドを Go に切り替える（`TYPLE_BACKEND=go`）

環境変数 `TYPLE_BACKEND=go` を付けてデプロイすると、次の3つの API を Go（Vercel の Go 関数 `api/go.go`）が答えます。本番でもプレビューでも同じです。外せば Next.js に戻ります。

- `/api/health`（GET）
- `/api/workspace`（GET・PUT）
- `/api/workspace/records`（POST・DELETE）

ログイン・端末・パックなど、ほかの API は Go に実装がないので、いつも Next.js です。Go のコードと、Next.js との違いは [docs/backend-compare.md](backend-compare.md) にあります。

## しくみ

- `next.config.mjs` が、ビルドのときに `TYPLE_BACKEND` を読みます。`go` なら、3つのパスを `/api/go?path=…` に回す rewrites を入れます（中身は `scripts/backendCompare.cjs`）。
- Go 関数は、実行時に `TYPLE_BACKEND=go`（または比較モードの `TYPLE_BACKEND_COMPARE=1`）がなければ、すべてに `404` を返します。
- 使える値は `next` と `go` だけです。ほかの値（`Go` など）はビルドが失敗します。打ち間違いで、思っていないバックエンドのまま本番に出ないようにするためです。
- Go の応答には `x-typle-backend: go` のヘッダーが付きます。どちらが答えたかは、これで確かめられます。

```bash
curl -sI https://typle.sparklingstadt.com/api/health | grep -i x-typle-backend
```

## 切り替える

Vercel の環境変数はデプロイごとに決まるので、変えたあとにデプロイし直します。

1. 本番の環境変数に足します（値は `go`）。

   ```bash
   vercel env add TYPLE_BACKEND production
   ```

2. いまの本番と同じ main のコミットで、本番にデプロイし直します（リリースの途中なら、その手順の `vercel deploy --prod --yes` がこれを兼ねます）。

   ```bash
   git switch main && git pull --ff-only && vercel deploy --prod --yes
   ```

3. 上の `curl` で `x-typle-backend: go` が返ることと、`npm run check:production` が通ることを確かめます。

## 戻す

- 急ぐとき: Vercel のダッシュボードの Instant Rollback で、Go にする前のデプロイに戻します。ビルドはありません。
- そのあと: 本番の環境変数から `TYPLE_BACKEND` を消して（`vercel env rm TYPLE_BACKEND production`）、もう一度デプロイします。消さないと、次のリリースでまた Go になります。

## 気をつけること

- ログインは Next.js（Auth.js）だけです。Go は Cookie（`github:…`・`password:<UUID>`・`ffpf:…` のアカウント）を読むだけで、Go に切り替えても ID・パスワードのアカウントの保存は通ります（`tests/fixtures/sessions.json` が確かめます）。
- `TYPLE_BACKEND=go` のときだけ、v2.54.0 までに発行したアプリのトークン（`Authorization: Bearer`、30日で切れる）が Go では通ります。Go の Bearer のコードは、Next.js の写しとして意図してそのままにしてあります。Next.js はもうトークンを発行せず、読みもしません（v2.55.0 から）。

- Go は、比べた時点の Next.js（v2.52.0）に動きをそろえてあります。そのあと Next.js の workspace や records の動きを変えたら、Go も直すまで Go に切り替えないでください。共通の fixture（`tests/fixtures/`）を変えると、`go test ./...` で食い違いが分かります。
- 公式パックのカタログを変えたら、`server/internal/workspace/packs/` も写し直します（`go test` の `TestPacksMatchApp` が知らせます）。
- 比較モード（`TYPLE_BACKEND_COMPARE=1`）と一緒に付けると、ヘッダーのないリクエストは Go、`x-typle-backend: next` は Next.js に行きます。
