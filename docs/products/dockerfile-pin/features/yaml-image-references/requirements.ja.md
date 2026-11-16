# YAMLのイメージ参照

[English](requirements.md)

## この文書について

この機能は、composeファイル、GitHub Actionsのファイル、GitLab CIのファイルからDockerイメージの参照を読み取り、各参照が書かれている場所にdigestを書き込みます。
`run`は読み取った参照をpinし、`check`はその参照について報告します。
親のProductは[dockerfile-pin](../../product.ja.md)です。

### スコープ

- ファイルの種類ごとに読むYAMLの位置と、そこでスキップする参照。
- すでにdigestがある参照の見分け方。
- digestをファイルの文字列へ書き込む方法。
- アンカー、エイリアス、マージキーを通して届く参照をどう読み、どう変えるか。

### スコープ外

- 対象ファイルを探して種類を決めること。対象ファイルの探索の機能に属します。
- レジストリからdigestを解決すること、`run`のdry-runと`--write`の出力、`check`の状態と出力。pinとcheckの機能に属します。
- 除外パターン、`--update`、`--min-age`。それぞれの機能に属します。
- Dockerfile。Dockerfileのイメージ参照の機能に属します。
- 変数の展開。composeの`${VAR}`とGitHub Actionsの`${{ }}`の式は書かれたまま読みます。`$`を含むGitLab CIの値は、後で述べるとおりスキップします。
- どの読み取る位置からも使われないアンカー付きのマッピング。たとえば、どのサービスもマージしない`x-*`のテンプレートです。そのイメージは読みません。

## ユーザーストーリー

### composeのサービスのイメージをpinする

composeファイルを管理する開発者として、各サービスの`image`に`run`でdigestを付けたい。サービスがpinしたイメージを使い続けるようにするためだ。

**受け入れ条件:**

シナリオ: サービスに`image`があるとき、タグの後ろにdigestを書く

- 前提: composeファイルに`services.web.image: node:20.11.1`がある。
- 操作: 開発者が`dockerfile-pin run --write -f docker-compose.yml`を実行する。
- 結果: その行が`image: node:20.11.1@sha256:<digest>`になり、ファイルのほかの行は変わらない。

シナリオ: サービスに`build`もあるとき、そのイメージはスキップする

- 前提: サービスに`build: .`と`image: myapp:latest`の両方がある。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: その参照は`has build directive`付きの`SKIP`として報告され、`run`はその行を変えない。

シナリオ: サービスに`image`がないとき、そのサービスについては何も報告しない

- 前提: サービスに`build: .`だけがある。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: そのサービスの結果は表示されない。

### GitHub Actionsのジョブとアクションのイメージをpinする

GitHub Actionsのファイルを管理する開発者として、コンテナ、サービス、`docker://`のステップのイメージをpinしたい。ジョブがpinしたイメージで動くようにするためだ。

**受け入れ条件:**

シナリオ: ワークフローの読み取る位置にイメージがあるとき、それぞれにdigestを付ける

- 前提: ワークフローに`jobs.test.container.image: node:24`、`jobs.test.services.db.image: postgres:18`、`uses: docker://ghcr.io/astral-sh/uv:latest`のステップがある。
- 操作: 開発者が`dockerfile-pin run --write`を実行する。
- 結果: 3つの値が`node:24@sha256:<digest>`、`postgres:18@sha256:<digest>`、`docker://ghcr.io/astral-sh/uv:latest@sha256:<digest>`になり、`docker://`の接頭辞は残る。

シナリオ: ステップがDockerイメージではないアクションを使うとき、読み取らない

- 前提: ステップに`uses: actions/checkout@v4`がある。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: そのステップの結果は表示されず、`run`もそのステップを変えない。

シナリオ: アクションがローカルのDockerfileからビルドするとき、スキップする

- 前提: `action.yml`に`runs.image: Dockerfile`がある。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: その参照は`local Dockerfile`付きの`SKIP`として報告される。

### GitLab CIのジョブのイメージをpinする

GitLab CIのファイルを管理する開発者として、ジョブ、`default`、隠しジョブの`image`と`services`をpinしたい。パイプラインがpinしたイメージで動くようにするためだ。

**受け入れ条件:**

シナリオ: ジョブがイメージを文字列かマッピングで書くとき、それぞれにdigestを付ける

- 前提: ファイルに`default.image.name: node:24`と、`services: [postgres:18, {name: redis:7, alias: cache}]`を持つジョブ`test`がある。
- 操作: 開発者が`dockerfile-pin run --write`を実行する。
- 結果: `node:24`、`postgres:18`、`redis:7`のそれぞれに、書かれている場所で`@sha256:<digest>`が付く。

シナリオ: イメージをCIの変数から組み立てるとき、スキップする

- 前提: ジョブに`image: $CI_REGISTRY_IMAGE:latest`がある。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: その参照は`contains CI variable`付きの`SKIP`として報告され、`run`はその参照を変えない。

シナリオ: CIコンポーネントに`spec:`のヘッダー文書があるとき、その後のジョブを読む

- 前提: ファイルが`spec:`の文書で始まり、`---`の後にジョブを持つ文書が続く。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: 2つ目の文書にあるジョブのイメージが報告される。

### 1行に書かれた複数の参照をpinする

フロースタイルでYAMLを書く開発者として、1行にある参照のそれぞれに、その参照のdigestを付けたい。ある参照がほかの参照の先頭部分と同じでも、壊れないようにするためだ。

**受け入れ条件:**

シナリオ: 1行にある2つの参照の先頭が同じとき、それぞれに自分のdigestを付ける

- 前提: GitLab CIのジョブに`services: [postgres:18, postgres]`がある。
- 操作: 開発者が`dockerfile-pin run --write`を実行する。
- 結果: その行が`services: [postgres:18@sha256:<postgres:18のdigest>, postgres@sha256:<postgresのdigest>]`になる。

### YAMLのアンカーで共有したイメージをpinする

YAMLのアンカーとマージキーで設定を共有する開発者として、サービスやジョブがそれらを通して受け取るイメージもcheckとpinの対象にしたい。共有したテンプレートのせいで、pinしていないイメージが見逃されないようにするためだ。

**受け入れ条件:**

シナリオ: サービスがマージキーからイメージを受け取るとき、そのイメージを書かれている場所でcheckする

- 前提: composeファイルの1行目に`x-alpine: &alpine`、2行目に`image: alpine:latest`があり、`<<: *alpine`だけを持つサービス`alpine-template`がある。
- 操作: 開発者が`dockerfile-pin check -f test.yml`を実行する。
- 結果: `alpine:latest`が2行目について`missing digest`付きの`FAIL`として報告される。

シナリオ: サービスがマージキーからイメージを受け取るとき、テンプレートにdigestを書く

- 前提: 上と同じcomposeファイルがある。
- 操作: 開発者が`dockerfile-pin run --write -f test.yml`を実行する。
- 結果: 2行目が`image: alpine:latest@sha256:<digest>`になり、`<<: *alpine`の行とファイルのほかの部分は変わらない。

シナリオ: 複数のサービスが同じテンプレートをマージするとき、digestは1回だけ書く

- 前提: サービス`a`と`b`が、どちらも`<<: *alpine`だけを持つ。
- 操作: 開発者が`dockerfile-pin run --write`を実行する。
- 結果: 2行目が`image: alpine:latest@sha256:<digest>`になり、その行の`@sha256:`はちょうど1つである。

シナリオ: 複数のサービスが同じテンプレートをマージするとき、`check`は定義について結果を1件表示する

- 前提: サービス`a`と`b`が、どちらも`<<: *alpine`だけを持ち、2行目に`image: alpine:latest`がある。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: `alpine:latest`の結果がちょうど1件表示され、2行目を指している。

シナリオ: サービスがマージキーと並べて自分の`image`を書くとき、自分のイメージを使う

- 前提: サービスに`<<: *alpine`と`image: node:20`があり、ほかに`*alpine`をマージするサービスはない。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: そのサービスについて`node:20`が報告され、2行目については何も報告されない。

シナリオ: マージキーが複数のテンプレートを並べるとき、前のテンプレートを優先する

- 前提: `image: a:1`を持つ`&a`と`image: b:1`を持つ`&b`のテンプレートと、`<<: [*a, *b]`だけを持つサービスがある。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: `a:1`が書かれている行について報告され、`b:1`は報告されない。

シナリオ: 値がエイリアスのとき、アンカーが書かれている場所でイメージを読む

- 前提: ワークフローに`jobs.a.container: &c {image: node:20}`と`jobs.b.container: *c`がある。または、composeファイルに`x-img: &img alpine:3.20`と、`image: *img`を持つサービスがある。
- 操作: 開発者が`dockerfile-pin run --write`を実行する。
- 結果: アンカーの行で`node:20`か`alpine:3.20`の後ろにdigestが書かれ、エイリアスの行は変わらない。

シナリオ: サービスがマージキーから`build`を受け取るとき、そのイメージはスキップする

- 前提: `build: .`を持つテンプレート`&built`と、`<<: *built`と`image: myapp:latest`を持つサービスがある。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: `myapp:latest`が`has build directive`付きの`SKIP`として報告される。

シナリオ: どの読み取る位置もテンプレートを使わないとき、そのイメージは読まない

- 前提: composeファイルに`image: alpine:latest`を持つ`x-unused: &unused`があり、どのサービスもそれをマージもエイリアスもしていない。
- 操作: 開発者が`dockerfile-pin check`を実行する。
- 結果: その行の結果は表示されない。

## KPI

ありません。dockerfile-pinは利用状況のデータを集めていないので、この機能の指標は測定していません。

## 機能要件

### 読み取る位置

**概要:** ファイルの種類ごとに決まった位置を読み、そこにあるスカラーを1つずつ参照として扱います。

**入力:** 種類が決まった対象ファイルの内容。

**出力:** 参照の一覧。各参照は、書かれたままの文字列、digestを除いたイメージ、あれば既存のdigest、行と列、スキップするかどうかとその理由を持ちます。

#### 適用条件

##### composeファイル

`services`の下にある各エントリの`image`を読みます。
トップレベルの`x-*`拡張フィールドなど、`services`の外は読みません。

##### GitHub Actionsのファイル

トップレベルに`jobs`があるファイルはワークフローとして、`runs`があるファイルはアクションとして読みます。
ワークフローでは、各ジョブの`container`（文字列か、マッピングの`image`）、`services`の下にある各エントリの`image`、`docker://`で始まる各ステップの`uses`を読みます。
アクションでは`runs.image`を読みます。

##### GitLab CIのファイル

ファイルにあるすべてのYAML文書を読みます。
ルートの`image`と`services`、およびルートにあるほかの各マッピングの`image`と`services`を読みます。`default`と、名前が`.`で始まる隠しジョブも含みます。
グローバルキーワードの`variables`、`workflow`、`include`、`stages`、`spec`はジョブとして読みません。
`image`は文字列か、`name`を読むマッピングです。`services`の各エントリも、文字列か、`name`を読むマッピングです。

##### アンカー、エイリアス、マージキー

読み取る位置は、composeとGitLab CIが解釈するのと同じYAMLの中で探します。
エイリアス（`*name`）は、そのアンカー（`&name`）が付いたノードを表します。値全体でも、サービス、ジョブ、マッピングでも同じです。
マージキー（`<<`）は、指しているマッピング、または指しているシーケンスの各マッピングのキーを、マージキーを持つマッピングへ加えます。[YAMLのマージキーの型](https://yaml.org/type/merge.html)に従い、マッピング自身のキーが優先され、シーケンスでは前のマッピングが後ろのマッピングより優先されます。
マージされたマッピングが、さらにマージキーを持つこともあります。
これはイメージの位置だけでなく、composeファイルの`build`にも当てはまります。

エイリアスやマージキーを通して届いた参照は、その文字列が書かれている行と列、つまりアンカーが付いたノードの中で報告し、変えます。
書かれた1つの参照に複数のサービスやジョブから届くときも、それは1つの参照です。結果は直す場所を示すものなので、`check`は定義の位置について結果を1件表示し、digestは1回だけ書きます。

GitHub Actionsのファイルにも同じ探し方を使います。
GitHub Actionsはアンカーとエイリアスを受け付けますが、マージキーは文書に載っていません（[changelog](https://github.blog/changelog/2025-09-18-actions-yaml-anchors-and-non-public-workflow-templates/)、[ドキュメント](https://docs.github.com/en/actions/reference/workflows-and-actions/reusing-workflow-configurations#yaml-anchors-and-aliases)）。そのため、実際にはエイリアスだけが使われます。

#### 例外時の振る舞い

##### スキップする参照

`build`を持つcomposeのサービスは、`has build directive`でスキップします。
`runs.image`が`docker://`で始まらないアクションは、`local Dockerfile`でスキップします。
`$`を含むGitLab CIの値は、`contains CI variable`でスキップします。
スキップした参照は`check`が報告し、`run`は変えません。

##### 正しくないYAML

YAMLとして正しくないファイルは、ファイル名付きのエラーとして報告し、ほかの対象ファイルの処理は続けます。

### 既存のdigest

**概要:** `@`を含む参照は、すでにdigestがあるものとして扱います。

**入力:** 参照の文字列。

**出力:** 最初の`@`より前の文字列がイメージで、後ろがdigestです。

### digestの書き込み

**概要:** ファイルの文字列の中で、各参照が書かれている場所にdigestを書き込みます。

**入力:** ファイルの内容、そこから読み取った参照、各参照に選んだdigest。

**出力:** 選んだ各参照が`<image>@<digest>`になったファイルの内容。

#### 適用条件

##### 変わるもの

変わるのは参照の文字列だけです。既存のdigestは置き換え、`docker://`の接頭辞は残し、引用符、コメント、インデント、ほかの行は書かれたままにします。

##### 1行にある参照

1行に複数の参照があるときは、各参照を書かれている列で変えます。そのため、ある参照の文字列にほかの参照のdigestが付くことはありません。

## データ要件

### 必要なデータ

- **イメージ参照:** 書かれたままの文字列、digestを除いたイメージ、既存のdigest、文字列が始まる行と列、`jobs.test.container.image`のような読める位置、スキップするときはその理由。コマンドの実行中だけ存在します。

### 関連

各イメージ参照は1つの対象ファイルに属します。`run`は、digestを除いたイメージで参照と解決したdigestを対応させます。

### 個人情報

ありません。この機能はリポジトリのファイルからイメージ名を読みます。

## 外部サービスへの依存

ありません。レジストリへの問い合わせは、pinとcheckの機能に属します。

## セキュリティ上の考慮事項

`run --write`は参照の文字列だけを変えるので、CIが使うコマンドや認証情報の参照など、ファイルのほかの部分の意味は変わりません。

## 未決事項

### 一部の使い方がスキップする共有の参照

- **確認したいこと:** 書かれた1つの参照に、`build`を持つcomposeのサービスと持たないサービスの両方から届くとき、スキップするかcheckするか。
- **候補とその根拠:** checkしてpinする。`build`を持たないサービスはそのイメージをpullするため。この場合を扱った資料はない。
- **決まるまで:** Designでは、どれか1つの使い方がスキップしなければcheckする。
- **判断する人:** メンテナー（azu）。
