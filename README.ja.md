# furiwake (振分) — AI エージェントルーティングプロキシ

[English](README.md)

[Claude Code](https://docs.anthropic.com/en/docs/claude-code) / [Codex CLI](https://github.com/openai/codex) と複数の AI プロバイダの間に入る軽量ルーティングプロキシ。システムプロンプトのマーカーでリクエストを自動振り分けします。チームエージェントでも単体でも動作します。

```
                         furiwake.yaml
                              |
  Claude Code            furiwake (:52860)             プロバイダ
 +-----------+     +-------------------------+
 |           |     |  @route:anthropic        | -----> Anthropic API  (passthrough)
 | Leader    | --> |  @route:codex            | -----> ChatGPT/Codex  (Responses API)
 | Coder     | --> |  @route:openai           | -----> OpenAI         (Chat Completions)
 | Researcher| --> |  @route:ollama           | -----> Ollama         (ローカル)
 |           |     |  @route:openrouter       | -----> OpenRouter     (Chat Completions)
 +-----------+     +-------------------------+
    Anthropic           API 変換                  OpenAI 互換
    Messages API        + SSE ストリーミング       APIs
```

## なぜ必要？

Claude Code のエージェントはデフォルトですべて Anthropic に接続します。でも、エージェントごとにプロバイダを変えたい場面は多いはず：

- リーダー → Anthropic（直接接続）
- コーダー → Codex や OpenAI
- リサーチャー → OpenRouter の無料モデル

エージェントファイルに `@route:` マーカーを書くだけ。Anthropic Messages API と OpenAI 互換 API の変換は furiwake が自動で行います。コード変更も複雑な設定も不要です。

### furiwake の特長

- **プロンプト内マーカールーティング** — `@route:` / `@model:` / `@reasoning:` をエージェントファイルに直接記述。別途ルーティング設定は不要。
- **ゼロ依存シングルバイナリ** — Go バイナリ 1 つと YAML 1 つで完結。Python も Docker も不要。
- **Codex ネイティブ** — OpenAI Chat Completions だけでなく ChatGPT Responses API (Codex) にも対応。

## クイックスタート

### 1. インストール

```bash
curl -fsSL https://github.com/HoshimuraYuto/furiwake/releases/latest/download/install.sh | bash
```

ソースからビルドする場合：

```bash
go build -o furiwake .
cp furiwake.yaml.example furiwake.yaml
```

### 2. 設定

`furiwake.yaml` を編集してプロバイダと API キーを設定します。

### 3. 起動

```bash
./furiwake
```

### 4. Claude Code から接続

```bash
export ANTHROPIC_BASE_URL=http://localhost:52860
claude
```

環境変数を永続化するには `~/.bashrc`（または `~/.zshrc`）に追記します：

```bash
echo 'export ANTHROPIC_BASE_URL=http://localhost:52860' >> ~/.bashrc
```

OpenAI や OpenRouter プロバイダを使う場合は API キーも追加：

```bash
echo 'export OPENAI_API_KEY="sk-proj-..."' >> ~/.bashrc
echo 'export OPENROUTER_API_KEY="sk-or-..."' >> ~/.bashrc
```

Codex プロバイダを使う場合は、先に Codex CLI でログインしておきます（`~/.codex/auth.json` が作成されます）：

```bash
codex
```

まとめて設定するなら：

```bash
cat >> ~/.bashrc << 'EOF'
export ANTHROPIC_BASE_URL=http://localhost:52860
export OPENAI_API_KEY="sk-proj-..."
export OPENROUTER_API_KEY="sk-or-..."
EOF

source ~/.bashrc
```

追記後は `source ~/.bashrc` で反映。

## Docker

furiwake をスタンドアロンコンテナとして起動し、複数の Docker コンテナ（devcontainer、エージェントコンテナなど）から共有して使う構成です。

### ファイル構成

```
docker/
├── Dockerfile          # install.sh で最新バイナリを取得
├── docker-compose.yml  # ポート 52860 公開 + furiwake-net ネットワーク作成
├── furiwake.yaml       # プロバイダ設定 — ここを編集
├── .env.example        # API キーのテンプレート
└── setup.sh            # 起動 / 更新ヘルパー
```

### セットアップ

```bash
cd docker

# 1. API キーを設定
cp .env.example .env
# .env を編集してキーを記入

# 2. プロバイダを設定
# 必要に応じて furiwake.yaml を編集

# 3. 起動（devcontainer を起動する前にホストで実行）
bash setup.sh
```

### Docker 環境での認証

Docker や devcontainer で動かす場合、プロバイダの認証タイプによって動作が異なります：

| 認証タイプ            | Docker での動作                                                                                                                |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `none`（passthrough） | Claude Code が自身の認証ヘッダーを送信し、furiwake はそのままリレーします。ホスト側の認証情報は不要。                          |
| `bearer`              | `.env` の環境変数で API キーを渡します。ホスト側のファイルは不要。                                                             |
| `codex`               | furiwake がボリュームマウント経由で**ホストの** `~/.codex/auth.json` を読み取ります。事前にホスト側で Codex にログインが必要。 |

Codex プロバイダを使う場合は、`docker/docker-compose.yml` のボリュームマウントをコメント解除してください：

```yaml
volumes:
  - ~/.codex:/root/.codex:ro
```

そしてホストマシンでログインします：

```bash
codex
```

### 他のコンテナから接続する

接続したい各コンテナの docker-compose.yml に `furiwake-net` を追加し、`ANTHROPIC_BASE_URL` をコンテナ名で指定します：

```yaml
# 接続側コンテナの docker-compose.yml
services:
  my-agent:
    environment:
      - ANTHROPIC_BASE_URL=http://furiwake:52860
    networks:
      - furiwake-net

networks:
  furiwake-net:
    external: true
```

> **注意:** コンテナ間は `http://furiwake:52860`（コンテナ名）、
> ホストからは `http://localhost:52860` を使います。

### メンテナンス

```bash
# .env の変更や compose 設定の変更を反映
bash setup.sh

# furiwake の新バージョンを取得（ビルドキャッシュをクリアして再ビルド）
bash setup.sh update
```

## 特徴

**ルーティング & API 変換**
| 機能 | 説明 |
|------|------|
| マーカーベースルーティング | システムプロンプト内の `@route:<provider>` でバックエンドを決定 |
| エージェント単位モデル上書き | `@model:<model>` でプロバイダのデフォルトモデルをリクエスト単位で上書き |
| Reasoning 制御 | `@reasoning:<level>` で reasoning effort を上書き（Codex/Responses） |
| API 変換 | Anthropic Messages API <-> OpenAI Chat Completions / ChatGPT Responses API |
| ストリーミング | 双方向の SSE ストリーム変換に完全対応 |

**運用**
| 機能 | 説明 |
|------|------|
| 複数の認証方式 | Bearer トークン、Codex (`~/.codex/auth.json`)、認証なし |
| リトライ＆バックオフ | 429 レスポンスに対する自動指数バックオフ（最大5回） |
| タイムアウト設定 | `timeout_seconds` で上流リクエストのタイムアウトを設定可能 |
| 監査ログ | `[HTTP-OUT]` で実際の HTTP リクエスト URL を全リクエスト記録 |
| デバッグファイルログ | 全レベルを `furiwake-debug.log` に出力；コンソールは INFO 以上のみ |

**配布**
| 機能 | 説明 |
|------|------|
| 単一バイナリ | Go 製、外部依存は `gopkg.in/yaml.v3` のみ |
| クロスプラットフォーム | Linux、macOS、Windows (amd64/arm64) |
| リリースアセット | Semantic Release でマルチプラットフォームバイナリ + `install.sh` を配布 |

## ルーティングの仕組み

エージェントのシステムプロンプトにマーカーを追加してルーティングを制御します：

```markdown
<!-- .claude/agents/implementer.md -->
<!-- @route:codex @model:gpt-5.3-codex @reasoning:high -->

You are a code implementation specialist...
```

動作サンプルが `.claude/agents/coder.md` に含まれています。

### マーカー

| マーカー             | 効果                                        | 例                  |
| -------------------- | ------------------------------------------- | ------------------- |
| `@route:<name>`      | 特定のプロバイダにルーティング              | `@route:codex`      |
| `@model:<name>`      | プロバイダのデフォルトモデルを上書き        | `@model:gpt-5-mini` |
| `@reasoning:<level>` | reasoning effort を上書き（`chatgpt` のみ） | `@reasoning:high`   |

### 解決ルール

```
                    @route マーカーあり？
                     /            \
                   あり            なし
                   /                \
          providers[name]     default_provider (anthropic)
                 |
         @model マーカーあり？
          /            \
        あり            なし
        /                \
  指定モデルを        providers[name].model
  使用               (furiwake.yaml から)
```

- `@reasoning` の許可値: `none | minimal | low | medium | high | xhigh`
- `@reasoning` 未指定時は設定ファイルの `providers.<name>.reasoning_effort` を使用

### リクエストフロー

```
  Claude Code                   furiwake                          プロバイダ
      |                            |                                 |
      |   POST /v1/messages        |                                 |
      |  (Anthropic 形式)          |                                 |
      |--------------------------->|                                 |
      |                            |  1. JSON パース                 |
      |                            |  2. @route / @model /           |
      |                            |     @reasoning マーカー検出     |
      |                            |  3. プロバイダ & モデル解決     |
      |                            |                                 |
      |                            |  [passthrough]                  |
      |                            |--- そのままリレー ------------->| Anthropic
      |                            |                                 |
      |                            |  [openai / chatgpt]             |
      |                            |--- リクエスト変換 ------------>| OpenAI / Codex
      |                            |<-- SSE レスポンス逆変換 -------| / Ollama / OpenRouter
      |                            |                                 |
      |   SSE ストリーム            |                                 |
      |  (Anthropic 形式)          |                                 |
      |<---------------------------|                                 |
```

## 設定

設定は `furiwake.yaml` に記述します（設定例: [furiwake.yaml.example](furiwake.yaml.example)）。トップレベルの項目はすべて**必須**：

| フィールド         | 説明                                    | 例                    |
| ------------------ | --------------------------------------- | --------------------- |
| `listen`           | バインドアドレス                        | `":52860"`            |
| `spoof_model`      | Claude Code に返すモデル名              | `"claude-sonnet-4-6"` |
| `default_provider` | `@route` マーカーなし時のフォールバック | `"anthropic"`         |
| `timeout_seconds`  | HTTP クライアントタイムアウト           | `300`                 |
| `providers`        | プロバイダ定義                          | (下記参照)            |

### プロバイダ

```yaml
providers:
  anthropic:
    type: passthrough
    url: "https://api.anthropic.com"
    auth:
      type: none

  codex:
    type: chatgpt
    url: "https://chatgpt.com/backend-api/codex/responses"
    model: "gpt-5.3-codex"
    reasoning_effort: "medium"
    auth:
      type: codex

  openai:
    type: openai
    url: "https://api.openai.com/v1/chat/completions"
    model: "gpt-5-mini"
    auth:
      type: bearer
      token_env: "OPENAI_API_KEY"

  ollama:
    type: openai
    url: "http://localhost:11434/v1/chat/completions"
    model: "qwen2.5-coder:32b"
    auth:
      type: none

  openrouter:
    type: openai
    url: "https://openrouter.ai/api/v1/chat/completions"
    model: "z-ai/glm-4.5-air:free"
    auth:
      type: bearer
      token_env: "OPENROUTER_API_KEY"
```

### プロバイダタイプ

| タイプ        | 説明                                                   |
| ------------- | ------------------------------------------------------ |
| `passthrough` | Anthropic API にリクエストをそのままリレー（変換なし） |
| `openai`      | OpenAI Chat Completions API 形式に変換                 |
| `chatgpt`     | ChatGPT Responses API 形式に変換                       |

### 認証タイプ

| タイプ   | 説明                                                                                         |
| -------- | -------------------------------------------------------------------------------------------- |
| `none`   | 認証なし                                                                                     |
| `bearer` | 環境変数から Bearer トークンを取得（`token_env` で指定）                                     |
| `codex`  | `~/.codex/auth.json` からトークンとアカウント ID を取得、`Chatgpt-Account-Id` ヘッダーを送信 |

## エンドポイント

| エンドポイント              | メソッド | 説明                                           |
| --------------------------- | -------- | ---------------------------------------------- |
| `/health`                   | GET      | ヘルスチェック                                 |
| `/v1/messages`              | POST     | Anthropic Messages API（メインエンドポイント） |
| `/v1/messages/count_tokens` | POST     | トークンカウント（パススルーまたは推定）       |

## 動作確認

furiwake 起動後、別ターミナルから確認できます：

```bash
# ヘルスチェック
curl -s http://localhost:52860/health

# Codex (ChatGPT Responses API)
curl -s http://localhost:52860/v1/messages \
  -H "content-type: application/json" \
  -d '{
    "model":"claude",
    "stream": true,
    "system":"@route:codex @model:gpt-5.1-codex-mini @reasoning:low",
    "messages":[{"role":"user","content":"hi"}]
  }'

# OpenAI (Chat Completions API)
curl -s http://localhost:52860/v1/messages \
  -H "content-type: application/json" \
  -d '{
    "model":"claude",
    "stream": true,
    "system":"@route:openai @model:gpt-5-mini",
    "messages":[{"role":"user","content":"hi"}]
  }'

# Ollama (ローカル)
curl -s http://localhost:52860/v1/messages \
  -H "content-type: application/json" \
  -d '{
    "model":"claude",
    "stream": true,
    "system":"@route:ollama @model:qwen2.5-coder:32b",
    "messages":[{"role":"user","content":"hi"}]
  }'

# OpenRouter
curl -s http://localhost:52860/v1/messages \
  -H "content-type: application/json" \
  -d '{
    "model":"claude",
    "stream": true,
    "system":"@route:openrouter @model:z-ai/glm-4.5-air:free",
    "messages":[{"role":"user","content":"hi"}]
  }'
```

## インストーラーオプション

```bash
# バージョン固定
curl -fsSL https://github.com/HoshimuraYuto/furiwake/releases/download/v1.2.3/install.sh | \
  bash -s -- --version v1.2.3

# バイナリと設定ファイルの配置先を変更
curl -fsSL https://github.com/HoshimuraYuto/furiwake/releases/latest/download/install.sh | \
  bash -s -- --bin-dir "$HOME/.local/bin" --config-dir "$HOME/.config/furiwake"
```

Linux ではインストーラーが systemd user service を自動的にセットアップします。

## ログ

2段階でログを出力します：

- **コンソール** — INFO、WARN、ERROR（カラー表示）
- **ファイル** (`furiwake-debug.log`) — DEBUG を含む全レベル

```
[HTTP-OUT] req=req_1739980000000 route=codex model=gpt-5.3-codex reasoning=high POST https://chatgpt.com/backend-api/codex/responses
```

ChatGPT/Codex プロバイダでは、DEBUG レベルで送信ペイロード (`[CODEX-REQ]`) と受信 SSE イベント (`[CODEX-SSE]`) も記録されます。

## 開発

開発環境には [Dev Containers](https://code.visualstudio.com/docs/devcontainers/containers) を使用しています。VS Code または GitHub Codespaces でリポジトリを開くと、Go・Node.js・必要なツールが自動的にセットアップされます。

```bash
make build            # ビルド
make run              # 実行
make test             # テスト
make fmt              # コードフォーマット（gofmt + prettier）
make cross            # 全プラットフォーム向けクロスコンパイル
make release-assets   # リリース用アセット作成（クロスビルド + チェックサム）
```

### バックグラウンド実行

```bash
./furiwake --config furiwake.yaml >/tmp/furiwake.log 2>&1 &
echo $!    # PID
kill <PID>
```

### デーモン運用（systemd user service）

`install.sh` で Linux に導入した場合：

```bash
systemctl --user status furiwake
systemctl --user restart furiwake
systemctl --user stop furiwake
journalctl --user -u furiwake -f
```

## プロジェクト構成

```
furiwake/
├── main.go                 # エントリーポイント、シグナル処理
├── config.go               # YAML 設定読み込み
├── server.go               # HTTP サーバー、エンドポイントルーティング、トークン推定
├── router.go               # @route:<name> 検出、プロバイダ解決
├── passthrough.go          # Anthropic パススルー処理
├── translate_request.go    # Anthropic → OpenAI リクエスト変換
├── translate_messages.go   # メッセージ・コンテンツブロック変換
├── translate_stream.go     # OpenAI SSE → Anthropic SSE 変換
├── translate_chatgpt.go    # ChatGPT Responses API 変換 + SSE
├── sse.go                  # SSE イベントパーサー
├── types.go                # 全構造体定義
├── auth.go                 # 認証処理 + 指数バックオフリトライ
├── logger.go               # コンソール + ファイルロガー
├── install.sh              # リリース installer（バイナリ + 設定 + systemd user service）
├── Makefile
└── furiwake.yaml.example
```

## 謝辞

furiwake は Claude Code プロキシエコシステムの先行プロジェクトからアイデアを得ています。以下のプロジェクトの先駆的な取り組みに感謝します：

- [claude-code-proxy](https://github.com/1rgs/claude-code-proxy) / [claude-code-proxy](https://github.com/fuergaosi233/claude-code-proxy) — Anthropic-to-OpenAI 変換の初期実装
- [claude-code-mux](https://github.com/9j/claude-code-mux) — マルチプロバイダフェイルオーバーを備えた高性能 Rust プロキシ
- [CCProxy](https://github.com/starbased-co/ccproxy) — LiteLLM 統合によるインテリジェントルーティング
- [LiteLLM](https://github.com/BerriAI/litellm) — 統合 API アクセスの価値を証明したユニバーサル LLM ゲートウェイ
- [Bifrost](https://github.com/maximhq/bifrost) — Go ネイティブゲートウェイの性能の可能性を示したプロジェクト
- [HydraTeams](https://github.com/Pickle-Pixel/HydraTeams) — Claude Code ツール完全対応のモデル非依存 Agent Teams プロキシ

## ライセンス

MIT
