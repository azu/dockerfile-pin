# dockerfile-pin

[English](product.md)

## 概要

dockerfile-pinは、Dockerfile、composeファイル、GitHub Actionsのファイル、GitLab CIのファイルに書かれたDockerイメージの参照へ`@sha256:<digest>`を付けるコマンドラインツールです。
`run`は各タグがレジストリで今指しているdigestを調べてタグの後ろに書き、`check`はdigestのない参照と、digestがレジストリに存在しない参照を報告します。
開発者の手元やCIで動き、レジストリの認証情報は`~/.docker/config.json`から読みます。

## 背景と課題

### 背景

`node:20`のようなDockerイメージのタグは書き換えられます。同じタグに、別の内容を後からpushできます（[Docker Community Forums](https://forums.docker.com/t/can-a-docker-hub-tag-have-its-content-changed/139358)）。
悪意のある第三者がレジストリへのアクセスを得ると、既存のタグに改ざんしたイメージをpushでき、タグだけを指定したビルドはそれをpullします。
[Trivyへの攻撃](https://www.aquasec.com/blog/trivy-supply-chain-attack-what-you-need-to-know/)のようなサプライチェーン攻撃は、イメージを公開するレジストリやパイプラインが現実に狙われることを示しています。
Docker Hubには[Immutable tags](https://docs.docker.com/docker-hub/repos/manage/hub-images/immutable-tags/)がありますが、設定するのはリポジトリのオーナーなので、イメージを使う側はこれに頼れません。

digestはイメージの内容のハッシュです。`@sha256:<digest>`でイメージを指定すると、レビューした内容のまま固定できます。lockfileがパッケージのintegrityのハッシュを固定するのと同じ考え方です。
`node:20.11.1@sha256:<digest>`のようにタグとdigestの両方を残すと、タグは人が読むために残り、Renovateは両方を更新できます。

### 解決したい課題

すべてのイメージのdigestを手で調べて書くのは、多くのDockerfile、composeファイル、CIのファイルにわたると現実的ではありません。
GitHub Actionsの`uses:`は[pinact](https://github.com/suzuki-shunsuke/pinact)がSHAでのpinを自動化しますが、Dockerイメージの参照には同じくらいシンプルなツールがありません。dockpinはメンテナンスが止まっていて、hadolintにはdigestを必須にするルールがありません。
CIで検査しないと、digestのない新しい参照が気付かれずにマージされます。また、打ち間違えたdigestや削除されたdigestは、`docker build`が失敗するまで分かりません。

## 対象ユーザー

### DockerとCIのファイルを管理する開発者

- **特徴:** 公開レジストリやプライベートレジストリのイメージを指定するDockerfile、composeファイル、GitHub Actionsのファイル、GitLab CIのファイルを持つリポジトリを管理しています。
- **課題:** digestを手で書かずにすべてのイメージ参照をdigestでpinし、pinしていない新しい参照がマージされるのを止めたいと考えています。
- **利用場面:** 既存のファイルをpinするために手元で`run --write`を1回実行し、プルリクエストごとにCIで`check`を実行し、その後のdigestの更新はRenovateに任せます。

## 提供価値

### 既存のファイルをまとめてpinする

- **受け取る人:** 既存のリポジトリにdigestでのpinを導入する開発者。
- **提供するもの:** `run --write`が、対象ファイルのすべての参照へ、タグを残したまま、タグが今指しているdigestを付けます。
- **理由:** digestを手で調べるのは規模が大きくなると続かないためです。タグを残すので、人もRenovateもタグを読み、更新し続けられます。

### pinしていない参照をCIで止める

- **受け取る人:** プルリクエストの開発者とレビュアー。
- **提供するもの:** `check`は、digestのない参照と、digestがレジストリに存在しない参照があると失敗します。
- **理由:** digestのない参照を新しく足せるなら、1回pinするだけでは足りないためです。存在も確かめるので、打ち間違えたdigestや削除されたdigestを、ビルドより先に見つけられます。検査はHEADリクエストを使うので、Docker Hubのpull rate limitを消費しません。

## 主要な概念

- **イメージ参照:** ファイルの中でDockerイメージを指す文字列です。`node:20`、`ghcr.io/org/app:v1`、`docker://alpine:3`などで、イメージ名と、省略できるタグとdigestからなります。
- **digest:** レジストリで1つのイメージの内容を識別する`sha256:<hex>`の値です。digest付きの参照は`image:tag@sha256:<hex>`と書きます。
- **pin（固定）:** タグを残したまま、タグが今指しているdigestを参照に付けることです。
- **対象ファイル:** dockerfile-pinが読むファイルです。種類はDockerfile、composeファイル、GitHub Actionsのファイル（ワークフローか`action.yml`）、GitLab CIのファイルのいずれかで、パスから決まります。
- **スキップ:** 読み取ったがpinもcheckもしない参照です。`scratch image`、`has build directive`、`contains CI variable`などの理由が付きます。
- **除外パターン:** `--ignore-images`か`.dockerfile-pin.yaml`で指定するglobパターンです。一致したイメージをpinとcheckの対象から外します。

## 機能の一覧

| 機能 | 説明 | 仕様 |
| --- | --- | --- |
| Dockerfileのイメージ参照 | `FROM`、`COPY --from=`、`RUN --mount=...,from=`からイメージ参照を読み取り、digestを書き込む |[Requirements](features/dockerfile-image-references/requirements.ja.md) |
| YAMLのイメージ参照 | composeファイル、GitHub Actionsのファイル、GitLab CIのファイルからイメージ参照を読み取り、digestを書き込む | [Requirements](features/yaml-image-references/requirements.ja.md) |
| 対象ファイルの探索 | `-f`、`--glob`、既定のパターンから対象ファイルを探し、各ファイルの種類を決める |[Requirements](features/target-file-discovery/requirements.ja.md) |
| pin | `run`がタグをdigestに解決し、pinしたファイルを表示するか書き込む |[Requirements](features/pinning/requirements.ja.md) |
| digestの更新 | `run --update`が既存のdigestを解決し直し、`--min-age`が最近ビルドされたイメージを飛ばす |[Requirements](features/digest-updates/requirements.ja.md) |
| check | `check`がdigestのない参照、レジストリにないdigest、`--verify-tag`では別のdigestを指すタグを報告する |[Requirements](features/checking/requirements.ja.md) |
| 除外と設定 | パターンでイメージを除外し、`.dockerfile-pin.yaml`から設定を読む |[Requirements](features/ignore-and-configuration/requirements.ja.md) |

## 成功の定義

dockerfile-pinを使うリポジトリのすべてのイメージ参照にdigestが付いていて、digestのない参照がCIによってマージされないこと。

## 開発の原則

- ツールはシンプルに保ちます。pinactがGitHub Actionsで行うように、digestを付けることと検査することに絞ります。
- 継続的なdigestの更新はRenovateに任せます。dockerfile-pinは最初のpinとCIでの検査を担い、`run --update`は手動で解決し直すためのもので、更新の仕組みではありません。
- メンテナンスが止まったライブラリではなく、Googleのgo-containerregistry（crane）のようにメンテナンスされているライブラリを土台にします。
- CIに入れるツール自体を安全に導入できるようにします。リリースにはaquaなどのインストーラーが確かめられるチェックサムを付け、GitHub Releasesのimmutabilityを有効にしています。

## SLA・SLI・SLO

### SLA

ありません。dockerfile-pinは利用者の手元やCIで動くコマンドラインツールで、可用性を保証できるような提供中のサービスがないためです。

### SLIとSLO

同じ理由でありません。利用状況のデータを集めていないので、測定している指標もありません。

## 未決事項

ありません。
