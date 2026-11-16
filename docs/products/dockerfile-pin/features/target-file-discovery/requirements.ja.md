# 対象ファイルの探索

[English](requirements.md)

## この文書について

この機能は、`run`と`check`がどのファイルを読むかと、各ファイルの種類を決めます。
親のProductは[dockerfile-pin](../../product.ja.md)です。

### スコープ

- `-f`、`--glob`、既定の探索からファイルを選ぶこと。
- ファイルがDockerfile、composeファイル、GitHub Actionsのファイル、GitLab CIのファイルのどれかを、パスから決めること。

### スコープ外

- ファイルから参照を読むこと。[Dockerfileのイメージ参照](../dockerfile-image-references/requirements.ja.md)と[YAMLのイメージ参照](../yaml-image-references/requirements.ja.md)に属します。
- ファイルの内容を見て種類を決めること。種類はパスだけで決めます。

## ユーザーストーリー

### ファイルを指定せずにリポジトリを検査する

開発者として、`check`と`run`に対象ファイルを自分で見つけてほしい。ファイルを並べずにCIで実行できるようにするためだ。

**受け入れ条件:**

シナリオ: gitリポジトリでファイルのオプションがないとき、追跡中のファイルと、無視されていない未追跡のファイルを探す

- 前提: gitリポジトリに`Dockerfile`、`services/api/Dockerfile.dev`、`docker-compose.yml`、`.github/workflows/ci.yml`、`.gitlab-ci.yml`と、`.gitignore`に書かれたディレクトリの中の`Dockerfile`がある。
- 操作: 開発者がリポジトリのルートで`dockerfile-pin check`を実行する。
- 結果: 最初の5つのファイルが読まれ、無視された`Dockerfile`は読まれない。

シナリオ: gitリポジトリの外でファイルのオプションがないとき、現在のディレクトリから既定のパターンで探す

- 前提: gitリポジトリではないディレクトリに`Dockerfile`と`node_modules/pkg/Dockerfile`がある。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: `Dockerfile`は読まれ、`node_modules`の下のものは読まれない。

シナリオ: 何も見つからないとき、コマンドはメッセージを出して失敗する

- 前提: ディレクトリに既定のパターンに一致するファイルがない。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: コマンドは`no Dockerfiles, compose files, GitHub Actions files, or GitLab CI files found`というエラーで終了する。

### 読むファイルを指定する

開発者として、1つのファイルかglobパターンを指定したい。既定の探索では見つからないファイルを読めるようにするためだ。

**受け入れ条件:**

シナリオ: `-f`がファイルを指定するとき、そのファイルだけを読む

- 前提: `path/to/Dockerfile`がある。
- 操作: 開発者が`dockerfile-pin run -f path/to/Dockerfile`を実行する。
- 結果: `--glob`も指定されていても、そのファイルだけが読まれる。

シナリオ: `-f`が存在しないファイルを指定するとき、コマンドは失敗する

- 前提: `missing.yml`は存在しない。
- 操作: 開発者が`dockerfile-pin check -f missing.yml`を実行する。
- 結果: コマンドは`file not found: missing.yml`で終了する。

シナリオ: `--glob`がファイルに一致するとき、それらのファイルを読む

- 前提: 複数のディレクトリに`Dockerfile`と`Dockerfile.prod`がある。
- 操作: 開発者が`dockerfile-pin check --glob '**/{Dockerfile,Dockerfile.*}'`を実行する。
- 結果: 一致したすべてのファイルが読まれる。何にも一致しないパターンは`no files matched pattern`で終了する。

### 各ファイルを正しい種類として読む

開発者として、ファイルの種類をパスから決めてほしい。各ファイルが正しい規則で読まれるようにするためだ。

**受け入れ条件:**

シナリオ: パスが渡されたとき、種類は規則の順に決まる

- 前提: パス`.github/workflows/ci.yml`、`action.yml`、`ci/build.gitlab-ci.yml`、`templates/docker-compose.yml`、`templates/deploy.yml`、`.gitlab/ci/test.yml`、`stack.yml`、`Dockerfile.dev`がある。
- 操作: それぞれを`-f`で渡す。
- 結果: 順に、GitHub Actions、GitHub Actions、GitLab CI、compose、GitLab CI、GitLab CI、compose、Dockerfileとして読まれる。

## KPI

ありません。dockerfile-pinは利用状況のデータを集めていません。

## 機能要件

### ファイルの選択

**概要:** `-f`は`--glob`より優先され、`--glob`は既定の探索より優先されます。

**入力:** `-f`と`--glob`のオプションと、現在のディレクトリ。

**出力:** ファイルの一覧か、エラー。

#### 適用条件

##### 既定の探索

既定のパターンは`**/{Dockerfile,Dockerfile.*,docker-compose*.yml,docker-compose*.yaml,compose.yml,compose.yaml,action.yml,action.yaml,.gitlab-ci.yml,.github/workflows/*.yml,.github/workflows/*.yaml}`です。
gitリポジトリの中では、gitが知っているファイルと、`.gitignore`が除外しない未追跡のファイルを、このパターンで絞り込みます。
gitが使えないとき、ディレクトリがリポジトリでないとき、その探索で何も見つからないときは、現在のディレクトリからファイルシステム上でこのパターンを探し、`node_modules`と`vendor`を飛ばします。

##### globの構文

`--glob`はdoublestarの構文を使い、`**`と波括弧の展開が使えます。

### 種類の決定

**概要:** 最初に一致した規則が種類を決めます。

**入力:** ファイルのパス。

**出力:** Dockerfile、compose、GitHub Actions、GitLab CIのいずれか。

#### 適用条件

| 順序 | パス | 種類 |
| --- | --- | --- |
| 1 | `.github/workflows/`の下の`.yml`か`.yaml` | GitHub Actions |
| 2 | `action.yml`か`action.yaml` | GitHub Actions |
| 3 | `.gitlab-ci.yml`で終わる名前 | GitLab CI |
| 4 | `compose.yml`、`compose.yaml`、`docker-compose*.yml`、`docker-compose*.yaml` | compose |
| 5 | ルートからの`templates/<name>.yml`か`templates/<name>/template.yml` | GitLab CI |
| 6 | `.gitlab/ci/`の下の`.yml`か`.yaml` | GitLab CI |
| 7 | そのほかの`.yml`か`.yaml` | compose |
| 8 | それ以外 | Dockerfile |

ファイル名は大文字と小文字を区別せずに比べます。
コンポーネントの行（5）は、リポジトリのルートからの相対パスにだけ一致します。絶対パスや`..`を含むパスには一致しません。
既定の探索に入っているのは`.gitlab-ci.yml`だけです。行3、5、6は、`-f`か`--glob`で指定したファイルに当てはまります。

## データ要件

### 必要なデータ

- **対象ファイル:** パスと種類。何も保存しません。

### 関連

ありません。

### 個人情報

ありません。

## 外部サービスへの依存

ありません。`git`コマンドはあれば使い、なくても探索は動きます。

## セキュリティ上の考慮事項

既定の探索は、依存のディレクトリなど、`.gitignore`が除外するファイルを読みません。

## 未決事項

ありません。
